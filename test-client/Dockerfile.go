FROM python:3.12-slim
RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*
WORKDIR /tests
COPY test-go.sh .
RUN chmod +x test-go.sh
CMD ["./test-go.sh"]
