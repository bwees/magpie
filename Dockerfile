FROM alpine:3.22 AS runtime
ARG TARGETPLATFORM
RUN apk add --no-cache tini ca-certificates ffmpeg
WORKDIR /app

COPY ${TARGETPLATFORM}/magpie /usr/local/bin/magpie
COPY web/build /app/web
RUN chmod +x /usr/local/bin/magpie

ENV MAGPIE_PORT=3000
ENV MAGPIE_DATA_DIR=/data
ENV MAGPIE_WEB_DIR=/app/web

VOLUME /data
EXPOSE 3000

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/magpie"]
