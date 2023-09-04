FROM golang:1.19 as builder

ARG VERSION
ARG GIT_DIR
ARG GIT_COMMIT
ARG GIT_BRANCH
ARG BUILD_DATE

COPY ./ /workspace

WORKDIR /workspace

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOPROXY=https://goproxy.cn,direct
RUN go mod download && go mod verify

# Build
RUN go build -gcflags "all=-N -l" -ldflags "-X $GIT_DIR/pkg/version.Version=$VERSION -X $GIT_DIR/pkg/version.BuildDate=$BUILD_DATE -X $GIT_DIR/pkg/version.GitCommit=$GIT_COMMIT -X $GIT_DIR/pkg/version.GitBranch=$GIT_BRANCH" -a -o manager main.go

RUN chmod +x /workspace/manager

FROM busybox:latest
WORKDIR /
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]