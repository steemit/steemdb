# Use an official Python runtime as a parent image
# Use Python 3.7 to ensure compatibility with steem library (pycrypto dependency)
FROM python:3.7-slim

# Set the working directory
WORKDIR /src

# Install dependencies
RUN apt-get update && apt-get install -y make gcc libssl-dev && \
    rm -rf /var/lib/apt/lists/*

# Install Python dependencies
# Note: steem library requires pycrypto which has compatibility issues with Python 3.9+
RUN pip install --no-cache-dir steem pymongo requests

# Copy the current directory contents into the container at /src
COPY . /src

# Run sync.py when the container launches
CMD ["python", "/src/sync.py"]
