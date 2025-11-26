## 1.
go get -u google.golang.org/grpc
go get -u google.golang.org/protobuf

## 2.
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

## 3.
protoc --go_out=. ./rpcx/proto/common_worker.proto
protoc --go-grpc_out=. ./rpcx/proto/common_worker.proto

## 4. 生成TLS证书

### 生成ca私钥
openssl genrsa -out ca.key 4096
### 自签名生成ca.crt 证书文件
### 如果在 Windows 使用 Git Bash 出现错误
### name is expected to be in the format /type0=value0/type1=value1/type2=... where characters may be escaped by \. This name is not in that format: ...
### 则需要在命令前加上
### MSYS_NO_PATHCONV=1
### 例如 MSYS_NO_PATHCONV=1 openssl ...
MSYS_NO_PATHCONV=1 openssl req -new -x509 -days 3650 -key ca.key -out ca.crt -subj "/CN=localhost"
### 生成server/client key file
openssl genrsa -out server.key 2048
openssl genrsa -out client.key 2048
### 生成server/client csr file
openssl req -new -key server.key -out server.csr -config TLS.md -extensions SAN
openssl req -new -key client.key -out client.csr -config TLS.md -extensions SAN
### 生成server/client crt file
### Generates server.crt which is the certChainFile for the server
openssl x509 -req -days 3650 -in server.csr -CA ca.crt -CAkey ca.key -set_serial 01 -out server.crt -extfile TLS.md -extensions SAN
openssl x509 -req -days 3650 -in client.csr -CA ca.crt -CAkey ca.key -set_serial 01 -out client.crt -extfile TLS.md -extensions SAN
### Generates server.pem which is the privateKeyFile for the Server
openssl pkcs8 -topk8 -nocrypt -in server.key -out server.pem


### 独立server.key进行SAN自签
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr -config TLS.md -extensions SAN
openssl x509 -req -days 3650 -in server.csr -set_serial 01 -signkey server.key -out server.crt -extfile TLS.md -extensions SAN
