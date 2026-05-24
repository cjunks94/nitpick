# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/nitpick .

FROM alpine:3.20
RUN apk add --no-cache github-cli ca-certificates && \
    addgroup -S nitpick && adduser -S nitpick -G nitpick
USER nitpick
COPY --from=build /out/nitpick /usr/local/bin/nitpick

# Railway sets $PORT and expects the server to bind to it. EXPOSE is purely
# documentation — Railway routes by $PORT regardless. GitHub Actions overrides
# CMD with the review args (see action.yml) so this default only applies to
# server deployments.
EXPOSE 8080
ENTRYPOINT ["nitpick"]
CMD ["serve"]
