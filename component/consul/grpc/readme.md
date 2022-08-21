## 1.
go get -u google.golang.org/grpc
go get -u google.golang.org/protobuf

## 2.
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

## 3.
protoc --go_out=. ./component/consul/grpc/idworker.proto
protoc --go-grpc_out=. ./component/consul/grpc/idworker.proto