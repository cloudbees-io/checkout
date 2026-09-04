FROM alpine:3.24

RUN apk --no-cache upgrade && \
    apk --no-cache --update add git git-lfs gpg less openssh patch && \
    git lfs install

COPY checkout /usr/local/bin/checkout

WORKDIR /cloudbees/home

ENTRYPOINT ["checkout"]
