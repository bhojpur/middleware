FROM moby/buildkit:v0.9.3
WORKDIR /middleware
COPY middleware README.md /middleware/
ENV PATH=/middleware:$PATH
ENTRYPOINT [ "/bhojpur/middleware" ]