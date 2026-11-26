FROM golang:1.27-alpine AS build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/server /app/server
VOLUME ["/app/pb_data"]
EXPOSE 8090
ENTRYPOINT ["/app/server", "serve", "--http=0.0.0.0:8090", "--dir=/app/pb_data"]
