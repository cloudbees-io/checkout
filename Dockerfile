FROM golang:1.23.1-alpine3.20 AS build

WORKDIR /work

COPY go.mod* go.sum* ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /usr/local/bin/checkout main.go

FROM alpine:3.21

RUN apk fix && \
    apk --no-cache --update add git git-lfs gpg less openssh patch && \
    git lfs install

COPY --from=build /usr/local/bin/checkout /usr/local/bin/checkout

WORKDIR /cloudbees/home

ENTRYPOINT ["checkout"]
