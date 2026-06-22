FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

RUN pip install --no-cache-dir Flask==3.1.3

WORKDIR /app

COPY tests/integration/mock_api/app.py /app/app.py

EXPOSE 8080

CMD ["python", "/app/app.py"]
