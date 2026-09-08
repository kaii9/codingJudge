FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/executor ./cmd/executor
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/upload-cases ./cmd/upload-cases
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/migrate ./cmd/migrate

FROM alpine:3.20

RUN apk add --no-cache ca-certificates docker-cli
WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /out/worker /app/worker
COPY --from=build /out/executor /app/executor
COPY --from=build /out/upload-cases /app/upload-cases
COPY --from=build /out/migrate /app/migrate
COPY migrations /app/migrations
EXPOSE 8080 8090
CMD ["/app/api"]
