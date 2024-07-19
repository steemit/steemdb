
# 🌟 Steem Account Data Updater

This repository contains a script to update and maintain Steem account data and related properties in MongoDB. It fetches data from the Steem blockchain and stores it in MongoDB, running scheduled updates to keep the data current.

## ✨ Features
- Fetches Steem account details and global properties.
- Stores data in MongoDB with structured logging and error handling.
- Supports batch requests to the Steem API for efficient data retrieval.
- Uses APScheduler for scheduled updates.

## 📋 Requirements
- Python 3.9
- MongoDB
- Docker (for containerized deployment)

## ⚙️ Configuration
Create a `config.json` file in the root directory with the following structure:

```json
{
    "STEEMD_URLS": ["https://api.steemit.com"],
    "MONGODB": "your_mongodb_connection_string"
}
```

The script also supports configuration via environment variables, which can be specified when running the Docker container:

- `STEEMD_URLS`: A comma-separated list of Steem node URLs.
- `MONGODB`: The MongoDB connection string.

## 🚀 Installation

1. Clone the repository:
   ```sh
   git clone https://github.com/your-repo/steem-account-updater.git
   cd steem-account-updater
   ```

2. Build the Docker image:
   ```sh
   docker build -t steem-history .
   ```

3. Run the Docker container with environment variables (if not using `config.json`):
   ```sh
   docker run -d --name steem-history      -e STEEMD_URLS="http://10.10.100.12:8080"      -e MONGODB="mongodb://10.10.100.30:27017"      steem-history
   ```

4. Run the Docker Contaner with copied config.json 

```sh
docker run -d --name name steem-history steem-history
```

## 📚 Usage
The script performs the following tasks:
1. Updates client information.
2. Updates global properties.
3. Loads mvest per account.
4. Updates transaction history.
5. Processes and inserts account details into MongoDB.

Logs can be monitored using:
```sh
docker logs --follow steem-history
```

## 🤝 Contributing
Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

Please ensure that you update tests as appropriate.

## 📜 License
[MIT](https://choosealicense.com/licenses/mit/)
