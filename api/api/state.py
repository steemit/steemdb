from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field
from decimal import Decimal, getcontext, InvalidOperation
from typing import Optional
import logging
from steem import Steem
from config import steemNODE, mongo_db
from datetime import datetime, timedelta
from pymongo import ASCENDING, DESCENDING

router = APIRouter()

# Initialize Steem connection with retries
try:
    steem = Steem(
        nodes=[steemNODE],
        timeout=20,
        num_retries=3,
        retry_timeout=10
    )
    logging.info(f"Connected to Steem node: {steemNODE}")
except Exception as e:
    logging.critical(f"Steem connection failed: {str(e)}")
    raise RuntimeError("Critical: Failed to connect to Steem node") from e


# ----------------------------
# MODELS
# ----------------------------

class BlockResponse(BaseModel):
    block_num: int = Field(..., example=96264041)
    head_block_id: str = Field(..., example="05bbc8ce427289c5fad0b62dc4e7fc08f3da4aa8")
    timestamp: str = Field(..., example="2025-06-05T12:33:18")
    source: str = Field(default="steem_blockchain")
    node: str = Field(default=steemNODE)

class TPSResponse(BaseModel):


    tps_1h: float = Field(..., example=1.2)
    tps_1d: float = Field(..., example=1.2)
    tps_1w: float = Field(..., example=1.2)
    tps_1m: float = Field(..., example=1.2)
    tps_all_time: float = Field(..., example=1.2)

class VestsToSteemResponse(BaseModel):
    vests: float = Field(..., example=1000000)
    steem: float = Field(..., example=1.234)
    steem_per_mvests: float = Field(..., example=1.234)
    timestamp: str = Field(..., example="2025-06-05T12:33:18")

# ----------------------------
# ENDPOINTS
# ----------------------------


@router.get("/getLastIrreversibleBlock", response_model=BlockResponse)
async def get_last_irreversible_block():
    """Get last irreversible block number."""
    try:
        props = steem.get_dynamic_global_properties()
        return BlockResponse(
            block_num=props["last_irreversible_block_num"],
            head_block_id=props["head_block_id"],
            timestamp=props["time"],
            source="steem_blockchain",
            node=steemNODE
        )
    except Exception as e:
        logging.error(f"Failed to get irreversible block: {str(e)}")
        raise HTTPException(status_code=503, detail="Blockchain data unavailable")

@router.get("/getLastReversibleBlock", response_model=BlockResponse)
async def get_last_reversible_block():
    """Get head block number (last reversible block)."""
    try:
        props = steem.get_dynamic_global_properties()
        return BlockResponse(
            block_num=props["head_block_number"],
            head_block_id=props["head_block_id"],
            timestamp=props["time"],
            source="steem_blockchain",
            node=steemNODE
        )
    except Exception as e:
        logging.error(f"Failed to get reversible block: {str(e)}")
        raise HTTPException(status_code=503, detail="Blockchain data unavailable")

@router.get("/getTPS", response_model=TPSResponse)
async def get_tps():        



    """Get Transactions Per Second (TPS) metrics."""
    try:
        tps_collection = mongo_db["tps_cache"]
        blocks_collection = mongo_db["blocks"]

        cached = tps_collection.find_one({"_id": "latest"})
        if cached:
            return TPSResponse(**{k: cached.get(k, 0.0) for k in ["tps_1h", "tps_1d", "tps_1w", "tps_1m", "tps_all_time"]})

        def calculate_tps(start: datetime, end: datetime) -> float:
            cursor = blocks_collection.find({
                "timestamp": {"$gte": start.isoformat(), "$lte": end.isoformat()}
            }, {"transactions": 1})
            tx_count = sum(len(block.get("transactions", [])) for block in cursor)
            duration = (end - start).total_seconds()
            return round(tx_count / duration, 4) if duration > 0 else 0.0

        now = datetime.utcnow()
        tps_data = {
            "tps_1h": calculate_tps(now - timedelta(hours=1), now),
            "tps_1d": calculate_tps(now - timedelta(days=1), now),
            "tps_1w": calculate_tps(now - timedelta(weeks=1), now),
            "tps_1m": calculate_tps(now - timedelta(days=30), now),
            "tps_all_time": 0.0
        }

        oldest = blocks_collection.find_one({}, sort=[("timestamp", ASCENDING)])
        newest = blocks_collection.find_one({}, sort=[("timestamp", DESCENDING)])
        if oldest and newest:
            t_start = datetime.fromisoformat(oldest["timestamp"])
            t_end = datetime.fromisoformat(newest["timestamp"])
            tps_data["tps_all_time"] = calculate_tps(t_start, t_end)

        tps_collection.replace_one({"_id": "latest"}, {**tps_data, "_id": "latest"}, upsert=True)
        return TPSResponse(**tps_data)

    except Exception as e:
        logging.error(f"TPS calculation failed: {str(e)}")
        raise HTTPException(status_code=503, detail="Failed to calculate TPS")


    """
    Convert VESTS to STEEM using current blockchain values.
    """
    try:
        print(f"\n[DEBUG] Attempting to convert {vests} VESTS to STEEM")
        print(f"[DEBUG] Using Steem node: {steemNODE}")
        
        # Get current blockchain properties
        print("[DEBUG] Fetching dynamic global properties...")
        props = steem.get_dynamic_global_properties()
        print(f"[DEBUG] Received props: {props.keys()}")
        
        # Extract and convert values
        print("[DEBUG] Parsing vesting values...")
        total_vesting_fund_steem = float(props['total_vesting_fund_steem'].split(" ")[0])
        total_vesting_shares = float(props['total_vesting_shares'].split(" ")[0])
        print(f"[DEBUG] Total vesting fund (STEEM): {total_vesting_fund_steem}")
        print(f"[DEBUG] Total vesting shares: {total_vesting_shares}")
        
        # Calculate conversion rate
        steem_per_vest = total_vesting_fund_steem / total_vesting_shares
        steem_per_mvests = steem_per_vest * 1000000
        steem_amount = vests * steem_per_vest
        
        print(f"[DEBUG] Conversion results:")
        print(f"  STEEM per VEST: {steem_per_vest}")
        print(f"  STEEM per MVEST: {steem_per_mvests}")
        print(f"  {vests} VESTS = {steem_amount} STEEM")
        
        return VestsToSteemResponse(
            vests=vests,
            steem=steem_amount,
            steem_per_mvests=steem_per_mvests,
            timestamp=props['time']
        )
        
    except Exception as e:
        error_msg = f"VESTS conversion failed: {str(e)}"
        print(f"\n[ERROR] {error_msg}")
        print(f"[ERROR] Exception type: {type(e).__name__}")
        if hasattr(e, 'args'):
            print(f"[ERROR] Exception args: {e.args}")
        raise HTTPException(
            status_code=503,
            detail=error_msg
        )
@router.get("/convertVestsToSteem", response_model=VestsToSteemResponse)
async def convert_vests_to_steem(vests: float):
    """Convert VESTS to STEEM using current blockchain values."""
    try:
        props = steem.get_dynamic_global_properties()
        
        # Handle both string and object formats
        if isinstance(props['total_vesting_fund_steem'], dict):
            vesting_fund = float(props['total_vesting_fund_steem']['amount']) / (10 ** props['total_vesting_fund_steem']['precision'])
            vesting_shares = float(props['total_vesting_shares']['amount']) / (10 ** props['total_vesting_shares']['precision'])
        else:
            vesting_fund = float(props['total_vesting_fund_steem'].split()[0])
            vesting_shares = float(props['total_vesting_shares'].split()[0])

        steem_per_vest = vesting_fund / vesting_shares
        
        return VestsToSteemResponse(
            vests=vests,
            steem=vests * steem_per_vest,
            steem_per_mvests=steem_per_vest * 1_000_000,
            timestamp=props['time']
        )
        
    except Exception as e:
        raise HTTPException(
            status_code=503,
            detail=f"Failed to convert VESTS to STEEM: {str(e)}"
        )
