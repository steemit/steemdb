# Use an official Python runtime as a parent image
FROM python:3.9-slim

# Set the working directory
WORKDIR /src

# Install dependencies
RUN apt-get update && apt-get install -y make gcc libssl-dev

# Install Python dependencies
RUN pip install steem pymongo requests

# Copy the current directory contents into the container at /src
COPY . /src

# Run sync.py when the container launches
CMD ["python", "/src/sync.py"]
