FROM golang:1.13 as build

ENV GO111MODULE=on

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build grpc_server.go

FROM gcr.io/distroless/base
COPY --from=build /app/grpc_server /

EXPOSE 50051

ENTRYPOINT ["/grpc_server"]
