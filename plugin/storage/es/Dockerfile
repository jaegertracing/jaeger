FROM python:3-stretch
RUN pip install elasticsearch elasticsearch-curator
COPY esCleaner.py /es-index-cleaner/
ENTRYPOINT ["python3", "/es-index-cleaner/esCleaner.py"]
