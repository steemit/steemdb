
# 🌟 Steem Account Data Updater

This repository contains a sophisticated script designed to update and maintain Steem account data and associated properties within MongoDB. It retrieves data from the Steem blockchain and ensures its currency through scheduled updates.

## ✨ Features
- **Efficient Data Retrieval**: Leverages batch requests to the Steem API for optimal data fetching.
- **Comprehensive Data Storage**: Stores detailed Steem account information and global properties in MongoDB.
- **Structured Logging and Error Handling**: Provides robust mechanisms for logging and error management.
- **Automated Scheduling**: Utilizes APScheduler to perform regular updates, ensuring data remains current.

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

Alternatively, configure via environment variables when running the Docker container:

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

3. Rename `config.json.example` to `config.json` if using the file for configuration:
   ```sh
   mv config.json.example config.json
   ```

4. Run the Docker container using environment variables (if not using `config.json`):
   ```sh
   docker run -d --name steem-history -e STEEMD_URLS="http://10.10.100.12:8080" -e MONGODB="mongodb://10.10.100.30:27017" steem-history
   ```

5. Run the Docker container with `config.json`:
   ```sh
   docker run -d --name steem-history -v $(pwd)/config.json:/app/config.json steem-history
   ```

## 📚 Usage
The script executes the following tasks:
1. **Client Information Update**: Refreshes the client data to maintain accuracy.
2. **Global Properties Update**: Keeps global blockchain properties up-to-date.
3. **Account MVests Load**: Fetches and updates the MVests (Million Vests) per account.
4. **Transaction History Update**: Ensures the transaction history is current.
5. **Account Details Processing**: Processes and inserts comprehensive account details into MongoDB.

Monitor logs using:
```sh
docker logs --follow steem-history
```

## 🤝 Contributing
Pull requests are welcome. For substantial changes, please open an issue to discuss your proposed modifications.

Please ensure that you update tests as necessary.

## 📜 License
This project is licensed under the MIT License - see the [LICENSE](https://choosealicense.com/licenses/mit/) file for details.
