package cmd

// Copyright (c) 2018 Bhojpur Consulting Private Limited, India. All rights reserved.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

var (
	host string
)

var webCmdOpts struct {
	Host             string
	Kubeconfig       string
	K8sNamespace     string
	K8sLabelSelector string
	K8sPodPort       string
	DialMode         string
}

// webCmd represents the base command when called without any subcommands
var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Launch this Middleware Manager in a web server hosted mode",
	Run: func(cmd *cobra.Command, args []string) {
		if verbose {
			log.SetLevel(log.DebugLevel)
			log.Debug("verbose logging enabled")
		}
	},
}

type dialMode string

const (
	dialModeHost       = "host"
	dialModeKubernetes = "kubernetes"
)

func init() {
	rootCmd.AddCommand(webCmd)

	middlewareHost := os.Getenv("MIDDLEWARE_HOST")
	if middlewareHost == "" {
		middlewareHost = "localhost:7777"
	}
	middlewareKubeconfig := os.Getenv("KUBECONFIG")
	if middlewareKubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.WithError(err).Warn("cannot determine user's home directory")
		} else {
			middlewareKubeconfig = filepath.Join(home, ".kube", "config")
		}
	}
	middlewareNamespace := os.Getenv("MIDDLEWARE_K8S_NAMESPACE")
	middlewareLabelSelector := os.Getenv("MIDDLEWARE_K8S_LABEL")
	if middlewareLabelSelector == "" {
		middlewareLabelSelector = "app.kubernetes.io/name=middleware"
	}
	middlewarePodPort := os.Getenv("MIDDLEWARE_K8S_POD_PORT")
	if middlewarePodPort == "" {
		middlewarePodPort = "7777"
	}
	dialMode := os.Getenv("MIDDLEWARE_DIAL_MODE")
	if dialMode == "" {
		dialMode = string(dialModeHost)
	}

	webCmd.PersistentFlags().StringVar(&webCmdOpts.DialMode, "dial-mode", dialMode, "dial mode that determines how we connect to Bhojpur Middleware. Valid values are \"host\" or \"kubernetes\" (defaults to MIDDLEWARE_DIAL_MODE env var).")
	webCmd.PersistentFlags().StringVar(&webCmdOpts.Host, "host", middlewareHost, "[host dial mode] Bhojpur Middleware host to talk to (defaults to MIDDLEWARE_HOST env var)")
	webCmd.PersistentFlags().StringVar(&webCmdOpts.Kubeconfig, "kubeconfig", middlewareKubeconfig, "[kubernetes dial mode] kubeconfig file to use (defaults to KUEBCONFIG env var)")
	webCmd.PersistentFlags().StringVar(&webCmdOpts.K8sNamespace, "k8s-namespace", middlewareNamespace, "[kubernetes dial mode] Kubernetes namespace in which to look for the Bhojpur Midleware pods (defaults to MIDDLEWARE_K8S_NAMESPACE env var, or configured kube context namespace)")
	// The following are such specific flags that really only matters if one doesn't use the stock helm charts.
	// They can still be set using an env var, but there's no need to clutter the CLI with them.
	webCmdOpts.K8sLabelSelector = middlewareLabelSelector
	webCmdOpts.K8sPodPort = middlewarePodPort
}

type closableGrpcClientConnInterface interface {
	grpc.ClientConnInterface
	io.Closer
}

func dial() (res closableGrpcClientConnInterface) {
	var err error
	switch webCmdOpts.DialMode {
	case dialModeHost:
		res, err = grpc.Dial(webCmdOpts.Host, grpc.WithInsecure())
	case dialModeKubernetes:
		res, err = dialKubernetes()
	default:
		log.Fatalf("unknown dial mode: %s", webCmdOpts.DialMode)
	}

	if err != nil {
		log.WithError(err).Fatal("cannot connect to Bhojpur Middleware server")
	}
	return
}

func dialKubernetes() (closableGrpcClientConnInterface, error) {
	kubecfg, namespace, err := getKubeconfig(webCmdOpts.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("cannot load kubeconfig %s: %w", webCmdOpts.Kubeconfig, err)
	}
	if webCmdOpts.K8sNamespace != "" {
		namespace = webCmdOpts.K8sNamespace
	}

	clientSet, err := kubernetes.NewForConfig(kubecfg)
	if err != nil {
		return nil, err
	}

	pod, err := findMiddlewarePod(clientSet, namespace, webCmdOpts.K8sLabelSelector)
	if err != nil {
		return nil, fmt.Errorf("cannot find Bhojpur Middleware pod: %w", err)
	}

	localPort, err := findFreeLocalPort()

	ctx, cancel := context.WithCancel(context.Background())
	readychan, errchan := forwardPort(ctx, kubecfg, namespace, pod, fmt.Sprintf("%d:%s", localPort, webCmdOpts.K8sPodPort))
	select {
	case err := <-errchan:
		cancel()
		return nil, err
	case <-readychan:
	}

	res, err := grpc.Dial(fmt.Sprintf("localhost:%d", localPort), grpc.WithInsecure())
	if err != nil {
		cancel()
		return nil, fmt.Errorf("cannot dial forwarded connection: %w", err)
	}

	return closableConn{
		ClientConnInterface: res,
		Closer:              func() error { cancel(); return nil },
	}, nil
}

type closableConn struct {
	grpc.ClientConnInterface
	Closer func() error
}

func (c closableConn) Close() error {
	return c.Closer()
}

func findFreeLocalPort() (int, error) {
	const (
		start = 30000
		end   = 60000
	)
	for p := start; p <= end; p++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			l.Close()
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free local port found")
}

// GetKubeconfig loads kubernetes connection config from a kubeconfig file
func getKubeconfig(kubeconfig string) (res *rest.Config, namespace string, err error) {
	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
		&clientcmd.ConfigOverrides{},
	)
	namespace, _, err = cfg.Namespace()
	if err != nil {
		return nil, "", err
	}

	res, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, namespace, err
	}

	return res, namespace, nil
}

// findMiddlewarePod returns the first pod we found for a particular component
func findMiddlewarePod(clientSet kubernetes.Interface, namespace, selector string) (podName string, err error) {
	pods, err := clientSet.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return "", err
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pod in %s with label component=%s", namespace, selector)
	}
	return pods.Items[0].Name, nil
}

// ForwardPort establishes a TCP port forwarding to a Kubernetes pod
func forwardPort(ctx context.Context, config *rest.Config, namespace, pod, port string) (readychan chan struct{}, errchan chan error) {
	errchan = make(chan error, 1)
	readychan = make(chan struct{}, 1)

	roundTripper, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		errchan <- err
		return
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, pod)
	hostIP := strings.TrimLeft(config.Host, "https://")
	serverURL := url.URL{Scheme: "https", Path: path, Host: hostIP}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, &serverURL)

	stopChan := make(chan struct{}, 1)
	fwdReadyChan := make(chan struct{}, 1)
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	forwarder, err := portforward.New(dialer, []string{port}, stopChan, fwdReadyChan, out, errOut)
	if err != nil {
		panic(err)
	}

	var once sync.Once
	go func() {
		err := forwarder.ForwardPorts()
		if err != nil {
			errchan <- err
		}
		once.Do(func() { close(readychan) })
	}()

	go func() {
		select {
		case <-readychan:
			// we're out of here
		case <-ctx.Done():
			close(stopChan)
		}
	}()

	go func() {
		for range fwdReadyChan {
		}

		if errOut.Len() != 0 {
			errchan <- fmt.Errorf(errOut.String())
			return
		}

		once.Do(func() { close(readychan) })
	}()

	return
}
