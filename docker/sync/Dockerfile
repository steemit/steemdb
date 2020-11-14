FROM python:3.6.2-slim

RUN apt-get update && apt-get install -y make gcc libssl-dev

RUN pip3 install steem pymongo

COPY ./docker/sync /src
ADD ./rds-combined-ca-bundle.pem /src/rds-combined-ca-bundle.pem

CMD ["python", "/src/sync.py"]
