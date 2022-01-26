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
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	verbose bool
	host    string
)

var rootCmdOpts struct {
	Verbose bool
	Host    string
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mdwctl",
	Short: "Bhojpur Middlectl is a command & control engine for middleware management services",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			log.SetLevel(log.DebugLevel)
			log.Debug("verbose logging enabled")
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

const (
	dialModeHost = "host"
)

func init() {
	middlewareHost := os.Getenv("MIDDLEWARE_HOST")
	if middlewareHost == "" {
		middlewareHost = "localhost:7777"
	}

	rootCmd.PersistentFlags().BoolVar(&rootCmdOpts.Verbose, "verbose", false, "en/disable verbose logging")
	rootCmd.PersistentFlags().StringVar(&rootCmdOpts.Host, "host", middlewareHost, "[host dial mode] Bhojpur Middleware host to talk to (defaults to MIDDLEWARE_HOST env var)")
	// The following are such specific flags that really only matters if one doesn't use the stock helm charts.
	// They can still be set using an env var, but there's no need to clutter the CLI with them.
}
