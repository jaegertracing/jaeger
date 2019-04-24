FROM python:3-alpine

# Temporary fix for https://github.com/jaegertracing/jaeger/issues/1494
RUN pip install urllib3==1.21.1

RUN pip install elasticsearch elasticsearch-curator
COPY ./mappings/* /mappings/
COPY esRollover.py /es-rollover/

ENTRYPOINT ["python3", "/es-rollover/esRollover.py"]
