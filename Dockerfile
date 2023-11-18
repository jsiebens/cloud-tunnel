FROM cgr.dev/chainguard/static:latest
COPY cloud-tunnel /cloud-tunnel
ENTRYPOINT ["/cloud-tunnel"]