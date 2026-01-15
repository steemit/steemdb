#!/bin/bash
# Script to stop MongoDB test container

CONTAINER_NAME="mongo-test"

if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "Stopping MongoDB test container..."
    docker stop ${CONTAINER_NAME} > /dev/null 2>&1
    docker rm ${CONTAINER_NAME} > /dev/null 2>&1
    echo "MongoDB test container stopped and removed."
else
    echo "MongoDB test container not running."
fi
