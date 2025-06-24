from pymongo import MongoClient
steemNODE = "https://api.justyy.com"
MONGO_URI = "mongodb://10.10.100.34:27017/"
MONGO_DB_NAME = "BlazeDB"
MONGO_COLLECTION_BLOCKS = "Blocks"
REDIS_HOST = "10.10.100.34"
REDIS_PORT = 6379
mongo_client = MongoClient(MONGO_URI)
mongo_db = mongo_client[MONGO_DB_NAME]
blocks_collection = mongo_db[MONGO_COLLECTION_BLOCKS]
