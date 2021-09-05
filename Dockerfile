FROM golang:1.17-alpine as build
WORKDIR /src
COPY go.* .
RUN go mod download
COPY . .
RUN go build -o app

FROM alpine:latest
RUN apk add --update qpdf && rm -rf /var/cache/apk/*
COPY --from=build /src/app /app
CMD ["/app"]