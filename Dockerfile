FROM golang:1.25.3-alpine3.22 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download -x
COPY . .
RUN go build -o /app/keda-domino -ldflags "-s -w" ./main.go

FROM alpine:3.22
RUN apk --no-cache -U upgrade && apk add --no-cache aws-cli
WORKDIR /app
COPY --from=build /app/keda-domino .
USER guest
CMD ["/app/keda-domino"]
