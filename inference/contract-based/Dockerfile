FROM python:3.11-slim

WORKDIR /app

# commitllm deps (torch, transformers, merkle, etc.)
COPY commitllm/requirements.txt /tmp/commitllm_req.txt
RUN pip install --no-cache-dir -r /tmp/commitllm_req.txt

# py/ deps (eth-keys, eth-abi, requests, fastapi)
COPY py/requirements.txt /tmp/py_req.txt
RUN pip install --no-cache-dir -r /tmp/py_req.txt

COPY commitllm/ /app/commitllm/
COPY py/        /app/py/

ENV PYTHONPATH=/app/py:/app
ENV HF_HOME=/hf_cache

ENTRYPOINT ["python", "-m", "pytest", "-v"]
