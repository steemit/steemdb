from fastapi import APIRouter, Query, HTTPException, Path
from fastapi.responses import JSONResponse
from fastapi.encoders import jsonable_encoder
from motor.motor_asyncio import AsyncIOMotorClient
from bson import ObjectId
from pydantic import BaseModel, Field
from typing import List, Optional, Union, Any
from datetime import datetime, timedelta

from config import MONGO_URI, MONGO_DB_NAME, MONGO_COLLECTION_BLOCKS

router = APIRouter()

try:
    client = AsyncIOMotorClient(MONGO_URI)
    db = client[MONGO_DB_NAME]
    blocks_collection = db[MONGO_COLLECTION_BLOCKS]
except Exception as e:
    print(f"Error connecting to MongoDB: {e}")
    # If the database connection fails, the application will not be able to start properly.
    raise

# --- Helper Function ---
def _format_ops(ops_list: List[list]) -> List[dict]:
    """Converts a list of ['op_name', {payload}] into a list of structured dicts."""
    if not ops_list:
        return []
    return [{"op_type": op[0], "payload": op[1]} for op in ops_list if isinstance(op, list) and len(op) == 2]

# --- Pydantic Models ---

class OperationDetail(BaseModel):
    op_type: str
    payload: dict

class Block(BaseModel):
    id: str = Field(..., alias="_id")
    block_num: int
    timestamp: datetime
    previous: Optional[str]
    witness: Optional[str]
    witness_signature: Optional[str]
    transactions_count: int
    transactions: Optional[List[dict]] = None

    class Config:
        allow_population_by_field_name = True
        orm_mode = True
        json_encoders = {ObjectId: str}

class BlocksResponse(BaseModel):
    page: int
    limit: int
    total_blocks: int
    blocks: List[Block]

class BlockDetailsResponse(BaseModel):
    block_num: int
    block_id: Optional[str]
    transactions_count: int
    transactions: Optional[List[dict]]
    witness: Optional[str]
    witness_signature: Optional[str]
    previous: Optional[str]
    timestamp: Optional[datetime]
    virtual_ops: List[OperationDetail]

    class Config:
        json_encoders = {ObjectId: str}

class OpsInBlockResponse(BaseModel):
    block_num: int
    all_ops: List[OperationDetail]

class VirtualOpsInBlockResponse(BaseModel):
    block_num: int
    virtual_ops: List[OperationDetail]

class TransactionCountResponse(BaseModel):
    start_block: int
    end_block: int
    total_transactions: int

class UserActivityResponse(BaseModel):
    username: str
    range_queried: str
    activities: List[dict]

class TransactionDetail(BaseModel):
    ref_block_num: int
    ref_block_prefix: int
    expiration: datetime
    signatures: List[str]
    extensions: List[Any]

class TransactionDetailResponse(BaseModel):
    transaction_id: str
    block_num: int
    transaction_num: int
    operations: List[OperationDetail]
    details: TransactionDetail


# --- API Endpoints ---

@router.get(
    "/getBlocks",
    summary="Get latest blocks from MongoDB with pagination",
    response_model=BlocksResponse,
)
async def get_blocks(
    limit: int = Query(30, ge=1, le=100, description="Number of blocks to return"),
    last_block_num: Optional[int] = Query(None, description="Last block_num from previous page for pagination"),
    with_transactions: bool = Query(False, description="Include transactions in the response"),
):
    pipeline = []
    if last_block_num is not None:
        pipeline.append({"$match": {"block_num": {"$lt": last_block_num}}})

    pipeline.extend([
        {"$sort": {"block_num": -1}},
        {"$limit": limit}
    ])

    project_stage = {
        "_id": {"$toString": "$_id"}, "block_num": 1, "timestamp": 1, "previous": 1,
        "witness": 1, "witness_signature": 1,
        "transactions_count": {"$size": {"$ifNull": ["$transactions", []]}},
    }
    if with_transactions:
        project_stage["transactions"] = 1
    pipeline.append({"$project": project_stage})

    cursor = blocks_collection.aggregate(pipeline)
    blocks_data = await cursor.to_list(length=limit)
    total_blocks = await blocks_collection.count_documents({})

    return BlocksResponse(
        page=1, limit=limit, total_blocks=total_blocks,
        blocks=[Block(**b) for b in blocks_data],
    )

@router.get(
    "/getBlockDetails",
    summary="Get details of a block by block_num from MongoDB",
    response_model=BlockDetailsResponse,
)
async def get_block_details(
    block_num: int = Query(..., description="The block number to retrieve"),
):
    block = await blocks_collection.find_one({"block_num": block_num})
    if not block:
        raise HTTPException(status_code=404, detail=f"Block {block_num} not found")

    block['virtual_ops'] = _format_ops(block.get('virtual_ops', []))

    return JSONResponse(content=jsonable_encoder(block, custom_encoder={ObjectId: str}))

@router.get(
    "/getBlockById/{block_id}",
    summary="Get details of a block by its unique block_id hash",
    response_model=BlockDetailsResponse,
)
async def get_block_by_id(
    block_id: str = Path(..., description="The unique hash ID of the block to retrieve")
):
    block = await blocks_collection.find_one({"block_id": block_id})
    if not block:
        raise HTTPException(status_code=404, detail=f"Block with id {block_id} not found")

    block['virtual_ops'] = _format_ops(block.get('virtual_ops', []))

    return JSONResponse(content=jsonable_encoder(block, custom_encoder={ObjectId: str}))

@router.get(
    "/getTransactionById/{transaction_id}",
    summary="Get a single transaction by its ID",
    response_model=TransactionDetailResponse,
)
async def get_transaction_by_id(
    transaction_id: str = Path(..., description="The unique ID of the transaction to retrieve")
):
    block_with_trx = await blocks_collection.find_one(
        {"transactions.transaction_id": transaction_id},
        {"block_num": 1, "transactions": {"$elemMatch": {"transaction_id": transaction_id}}}
    )

    if not block_with_trx or not block_with_trx.get("transactions"):
        raise HTTPException(status_code=404, detail=f"Transaction with ID {transaction_id} not found")

    trx_data = block_with_trx["transactions"][0]
    block_num = block_with_trx["block_num"]
    transaction_num = -1 # Default value

    original_block = await blocks_collection.find_one(
        {"block_num": block_num},
        {"transactions.transaction_id": 1} 
    )

    if original_block and original_block.get("transactions"):
        trx_ids = [trx.get("transaction_id") for trx in original_block["transactions"]]
        try:
            transaction_num = trx_ids.index(transaction_id)
        except ValueError:
            pass

    response = TransactionDetailResponse(
        transaction_id=trx_data.get("transaction_id"),
        block_num=block_num,
        transaction_num=transaction_num,
        operations=_format_ops(trx_data.get("operations", [])),
        details=TransactionDetail(
            ref_block_num=trx_data.get("ref_block_num"),
            ref_block_prefix=trx_data.get("ref_block_prefix"),
            expiration=trx_data.get("expiration"),
            signatures=trx_data.get("signatures", []),
            extensions=trx_data.get("extensions", [])
        )
    )

    return response

@router.get(
    "/getOpsInBlock",
    summary="Get all operations (regular and virtual) in a specific block",
    response_model=OpsInBlockResponse,
)
async def get_ops_in_block(block_num: int = Query(..., description="The block number to inspect")):
    block = await blocks_collection.find_one(
        {"block_num": block_num},
        {"transactions.operations": 1, "virtual_ops": 1}
    )
    if not block:
        raise HTTPException(status_code=404, detail=f"Block {block_num} not found")

    all_ops = _format_ops(block.get("virtual_ops", []))
    for tx in block.get("transactions", []):
        all_ops.extend(_format_ops(tx.get("operations", [])))

    return OpsInBlockResponse(block_num=block_num, all_ops=all_ops)


@router.get(
    "/getVirtualOpsInBlock",
    summary="Get only the virtual operations in a specific block",
    response_model=VirtualOpsInBlockResponse,
)
async def get_virtual_ops_in_block(block_num: int = Query(..., description="The block number to inspect")):
    block = await blocks_collection.find_one({"block_num": block_num}, {"virtual_ops": 1})
    if not block:
        raise HTTPException(status_code=404, detail=f"Block {block_num} not found")

    virtual_ops = _format_ops(block.get("virtual_ops", []))

    return VirtualOpsInBlockResponse(block_num=block_num, virtual_ops=virtual_ops)


@router.get(
    "/getTransactionCountInRange",
    summary="Get the total number of transactions in a range of blocks",
    response_model=TransactionCountResponse,
)
async def get_transaction_count_in_range(
    start_block: int = Query(..., description="The starting block number of the range"),
    end_block: int = Query(..., description="The ending block number of the range"),
):
    pipeline = [
        {"$match": {"block_num": {"$gte": start_block, "$lte": end_block}}},
        {"$group": {
            "_id": None,
            "total_transactions": {"$sum": {"$size": {"$ifNull": ["$transactions", []]}}}
        }}
    ]
    result = await blocks_collection.aggregate(pipeline).to_list(length=1)
    total_transactions = result[0]['total_transactions'] if result else 0

    return TransactionCountResponse(
        start_block=start_block,
        end_block=end_block,
        total_transactions=total_transactions
    )

@router.get(
    "/getBlocksInRange",
    summary="Get full details for all blocks within a specified range",
    response_model=List[BlockDetailsResponse],
)
async def get_blocks_in_range(
    start_block: int = Query(..., description="The starting block number of the range"),
    end_block: int = Query(..., description="The ending block number of the range"),
    limit: int = Query(100, ge=1, le=1000, description="Max number of blocks to return")
):
    cursor = blocks_collection.find({
        "block_num": {"$gte": start_block, "$lte": end_block}
    }).sort("block_num", 1).limit(limit)

    blocks = await cursor.to_list(length=limit)
    if not blocks:
        raise HTTPException(status_code=404, detail="No blocks found in the specified range.")

    for block in blocks:
        block['virtual_ops'] = _format_ops(block.get('virtual_ops', []))

    return JSONResponse(content=jsonable_encoder(blocks, custom_encoder={ObjectId: str}))


@router.get(
    "/searchUserActivity",
    summary="Search for a user's activity in a date or block range",
    response_model=UserActivityResponse,
)
async def search_user_activity(
    username: str = Query(..., description="The username to search for"),
    start_date: Optional[datetime] = Query(None, description="Start timestamp (UTC)"),
    end_date: Optional[datetime] = Query(None, description="End timestamp (UTC)"),
    start_block: Optional[int] = Query(None, description="Start block number"),
    end_block: Optional[int] = Query(None, description="End block number"),
):
    match_stage = {}
    range_queried = ""

    if start_date and end_date:
        match_stage = {"timestamp": {"$gte": start_date, "$lte": end_date}}
        range_queried = f"Date range: {start_date.isoformat()} to {end_date.isoformat()}"
    elif start_block is not None and end_block is not None:
        match_stage = {"block_num": {"$gte": start_block, "$lte": end_block}}
        range_queried = f"Block range: {start_block} to {end_block}"
    else:
        raise HTTPException(status_code=400, detail="Must provide either date range or block range")

    user_fields = [
        "owner", "voter", "author", "producer", "from", "to", "current_owner", "open_owner",
        "seller", "buyer", "delegator", "delegatee", "account", "witness"
    ]
    or_conditions = [{f"op_payload.{field}": username} for field in user_fields]

    pipeline = [
        {"$match": match_stage},
        {"$project": {
            "_id": 0,
            "block_num": 1,
            "timestamp": 1,
            "all_ops": {"$concatArrays": [
                {"$ifNull": ["$virtual_ops", []]},
                {"$reduce": {
                    "input": {"$ifNull": ["$transactions", []]},
                    "initialValue": [],
                    "in": {"$concatArrays": ["$$value", "$$this.operations"]}
                }}
            ]}
        }},
        {"$unwind": "$all_ops"},
        {"$project": {
            "block_num": 1, "timestamp": 1, "op_type": {"$arrayElemAt": ["$all_ops", 0]},
            "op_payload": {"$arrayElemAt": ["$all_ops", 1]}
        }},
        {"$match": {"$or": or_conditions}},
        {"$limit": 500}
    ]

    activities = await blocks_collection.aggregate(pipeline).to_list(length=500)

    encoded_activities = jsonable_encoder(activities)

    return UserActivityResponse(username=username, range_queried=range_queried, activities=encoded_activities)
