FROM golang:1.20.5-alpine3.18 AS build

WORKDIR /work

COPY go.mod* go.sum* ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /usr/local/bin/actions-checkout main.go

FROM alpine:3.18

RUN apk fix && \
    apk --no-cache --update add git git-lfs gpg less openssh patch && \
    git lfs install

COPY --from=build /usr/local/bin/actions-checkout /usr/local/bin/actions-checkout

WORKDIR /cloudbees/home

ENTRYPOINT ["actions-checkout"]
