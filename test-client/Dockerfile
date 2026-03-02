FROM python:3.12-slim
RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*
WORKDIR /test
COPY test.sh .
RUN chmod +x test.sh
CMD ["./test.sh"]
