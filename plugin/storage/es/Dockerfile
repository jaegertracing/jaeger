FROM python:3-alpine3.11

COPY requirements-build.txt requirements-build.txt
COPY requirements.txt requirements.txt

RUN pip install -r requirements-build.txt
RUN pip install -r requirements.txt

COPY esCleaner.py /es-index-cleaner/

ENTRYPOINT ["python3", "/es-index-cleaner/esCleaner.py"]
