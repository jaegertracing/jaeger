FROM python:3-alpine3.11

# Temporary fix for https://github.com/jaegertracing/jaeger/issues/1494
RUN pip install urllib3==1.24.3

RUN pip install elasticsearch elasticsearch-curator
COPY esCleaner.py /es-index-cleaner/

ENTRYPOINT ["python3", "/es-index-cleaner/esCleaner.py"]
