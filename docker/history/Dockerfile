# Use an official Python runtime as a parent image
FROM python:3.9-slim

# Install system dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends git make gcc libssl-dev libgmp-dev python3-dev libxml2-dev libxslt1-dev zlib1g-dev && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install Python packages
RUN pip install --no-cache-dir steem pymongo apscheduler requests tenacity

# Copy the application code into the container
COPY . /src

# Set the working directory
WORKDIR /src

# Command to run the application
CMD ["python", "history.py"]
