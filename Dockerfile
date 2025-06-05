ARG UID=1234
ARG GID=1234

FROM golang:1.23.9-alpine3.22 AS build

ARG UID
ARG GID

WORKDIR /work

COPY go.mod* go.sum* ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /usr/local/bin/checkout main.go

FROM alpine:3.21

ARG UID
ARG GID

# Create user and group
RUN addgroup -g ${GID} -S checkoutgroup
RUN adduser -u ${UID} -S checkoutuser -G checkoutgroup

RUN apk fix && \
    apk --no-cache --update add git git-lfs gpg less openssh patch && \
    git lfs install

COPY --from=build /usr/local/bin/checkout /usr/local/bin/checkout

WORKDIR /cloudbees/home

USER ${UID}:${GID}

ENTRYPOINT ["checkout"]
