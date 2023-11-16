FROM ubuntu:latest
RUN apt update -y && apt install -y ca-certificates

WORKDIR /app
COPY ip2region /app/ip2region

EXPOSE 8080
CMD ["/app/ip2region", "-p", "8080"]
