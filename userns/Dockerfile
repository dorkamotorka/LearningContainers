FROM alpine:3.18.0

WORKDIR /tmp
RUN adduser -D appuser
USER appuser

RUN touch file
CMD ls -l .
