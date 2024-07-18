FROM python:3.9-slim

# Install dependencies
RUN apt-get update && \
    apt-get install -y git make gcc libssl-dev libgmp-dev python3-dev libxml2-dev libxslt1-dev zlib1g-dev && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install Python packages
RUN pip install steem pymongo apscheduler requests tenacity

# Copy the application code into the container
COPY . /src



# Copy config.json into the container
COPY ./config.json /src/config.json

# Set the working directory
WORKDIR /src

# Command to run the application
CMD ["python", "history.py"]
