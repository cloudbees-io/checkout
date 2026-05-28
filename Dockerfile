FROM alpine:3.23

RUN apk fix && \
    apk --no-cache --update add git git-lfs gpg less openssh patch && \
    git lfs install

COPY checkout /usr/local/bin/checkout

WORKDIR /cloudbees/home

ENTRYPOINT ["checkout"]
