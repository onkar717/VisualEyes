FROM python:3.11-slim

WORKDIR /app

# System deps for kubernetes client and scipy
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc g++ libffi-dev \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy service code and shared runbooks
COPY . /app/ai_sre
COPY ../server/rca/runbooks /app/runbooks

ENV RUNBOOKS_DIR=/app/runbooks
ENV AI_SRE_HOST=0.0.0.0
ENV AI_SRE_PORT=8001
ENV PYTHONPATH=/app

EXPOSE 8001

HEALTHCHECK --interval=30s --timeout=10s --start-period=20s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8001/health')"

CMD ["python", "-m", "uvicorn", "ai_sre.main:app", "--host", "0.0.0.0", "--port", "8001"]
