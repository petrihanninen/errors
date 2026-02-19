FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o errors ./cmd/errors

FROM alpine:3.21
RUN apk add --no-cache ca-certificates git grep nodejs npm
RUN npm install -g @anthropic-ai/claude-code
RUN git config --global user.name "Errors Bot" && \
    git config --global user.email "errors-bot@noreply.github.com"
COPY --from=builder /app/errors /usr/local/bin/errors
ENTRYPOINT ["errors"]
