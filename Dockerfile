FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
# go.sum may not exist for stdlib-only modules; touch it to satisfy COPY
RUN touch go.sum
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /web-gateway .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /web-gateway /usr/local/bin/web-gateway
EXPOSE 8080
ENTRYPOINT ["web-gateway"]
