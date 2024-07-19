# Steem Account Data Updater

This repository contains a script to update and maintain Steem account data and related properties in MongoDB. It fetches data from the Steem blockchain and stores it in MongoDB, running scheduled updates to keep the data current.

## Features
- Fetches Steem account details and global properties.
- Stores data in MongoDB with structured logging and error handling.
- Supports batch requests to the Steem API for efficient data retrieval.
- Uses APScheduler for scheduled updates.

## Requirements
- Python 3.9
- MongoDB
- Docker (for containerized deployment)

## Configuration
Create a `config.json` file in the root directory with the following structure:

```json
{
    "STEEMD_URLS": ["https://api.steemit.com"],
    "MONGODB": "your_mongodb_connection_string"
}
