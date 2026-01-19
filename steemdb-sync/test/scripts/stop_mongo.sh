#!/bin/bash
# Script to stop MongoDB test container and clean up volume

CONTAINER_NAME="mongo-test"
VOLUME_NAME="mongo-test-data"

if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "Stopping MongoDB test container..."
    docker stop ${CONTAINER_NAME} > /dev/null 2>&1
    docker rm ${CONTAINER_NAME} > /dev/null 2>&1
    echo "MongoDB test container stopped and removed."
else
    echo "MongoDB test container not running."
fi

# Clean up volume if it exists
if docker volume ls --format '{{.Name}}' | grep -q "^${VOLUME_NAME}$"; then
    echo "Removing volume ${VOLUME_NAME}..."
    docker volume rm ${VOLUME_NAME} > /dev/null 2>&1
    echo "Volume ${VOLUME_NAME} removed."
else
    echo "Volume ${VOLUME_NAME} does not exist."
fi
