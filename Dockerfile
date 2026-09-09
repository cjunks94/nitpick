# syntax=docker/dockerfile:1
# Digest-pinned so a rebuilt tag cannot change the toolchain under us;
# Dependabot's docker ecosystem moves the digest when the tag is rebuilt.
FROM golang:1.24-alpine@sha256:8bee1901f1e530bfb4a7850aa7a479d17ae3a18beb6e09064ed54cfd245b7191 AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
# A failed download must fail the build, not fall through to a go build
# that fails later with a less useful error.
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/nitpick .

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
# github-cli is only used by the `review` subcommand (the GitHub Action
# path shells out to gh); `serve` talks REST with an installation token
# and never needs it. It stays in the one image so action.yml and the
# Railway deploy build the same artifact.
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
