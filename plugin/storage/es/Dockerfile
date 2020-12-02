FROM python:3-alpine3.11

COPY esCleaner.py /es-index-cleaner/
COPY requirements.txt /es-index-cleaner/

RUN pip install -r /es-index-cleaner/requirements.txt

ENTRYPOINT ["python3", "/es-index-cleaner/esCleaner.py"]
