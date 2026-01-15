#!/bin/bash
# Script to start MongoDB container for testing

set -e

CONTAINER_NAME="mongo-test"
MONGO_USERNAME="${MONGO_USERNAME:-admin}"
MONGO_PASSWORD="${MONGO_PASSWORD:-123456}"
MONGO_PORT="${MONGO_PORT:-27017}"

echo "Starting MongoDB test container..."

# Check if container already exists
if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "Container ${CONTAINER_NAME} already exists. Removing..."
    docker rm -f ${CONTAINER_NAME} > /dev/null 2>&1 || true
fi

# Start MongoDB container
docker run -d \
    --name ${CONTAINER_NAME} \
    -p ${MONGO_PORT}:27017 \
    -e MONGO_INITDB_ROOT_USERNAME=${MONGO_USERNAME} \
    -e MONGO_INITDB_ROOT_PASSWORD=${MONGO_PASSWORD} \
    mongo:4.4

echo "Waiting for MongoDB to be ready..."
sleep 5

# Wait for MongoDB to be ready
for i in {1..30}; do
    if docker exec ${CONTAINER_NAME} mongo --quiet --eval "db.adminCommand('ping')" --username ${MONGO_USERNAME} --password ${MONGO_PASSWORD} --authenticationDatabase admin > /dev/null 2>&1; then
        echo "MongoDB is ready!"
        echo ""
        echo "Connection details:"
        echo "  URI: mongodb://${MONGO_USERNAME}:${MONGO_PASSWORD}@127.0.0.1:${MONGO_PORT}/steemdb_test?authSource=admin"
        echo "  Container: ${CONTAINER_NAME}"
        echo ""
        echo "To stop the container:"
        echo "  docker stop ${CONTAINER_NAME} && docker rm ${CONTAINER_NAME}"
        exit 0
    fi
    sleep 1
done

echo "MongoDB failed to start within 30 seconds"
docker logs ${CONTAINER_NAME}
exit 1
