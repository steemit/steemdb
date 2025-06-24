from fastapi import FastAPI, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from starlette.middleware.base import BaseHTTPMiddleware
from redis.asyncio import Redis
from pydantic import BaseModel
from contextlib import asynccontextmanager
import config

# Import your routers
from api.blocks import router as blocks_router
#from api.operations import router as operations_router
#from api.state import router as state_router
#from api.accounts import router as accounts_router
#from api.content import router as content_router
#from api.communities import router as communities_router
#from api.feeds import router as feeds_router
#from api.market import router as market_router
#from api.governance import router as governance_router

# --- Declarative Cache Rules ---
# The key is the full path, and the value is the cache expiry time in seconds.
# Use 'None' to explicitly skip caching for a path.
CACHE_RULES = {
    "/blocks/getBlockDetails": 300,  # 5 minutes 
    "/blocks/getBlocks": None,       # Skip caching
    
}


redis_client: Redis = None

@asynccontextmanager
async def lifespan(app: FastAPI):
    """
    Manages the Redis connection lifecycle.
    """
    global redis_client
    print("Connecting to Redis...")
    redis_client = Redis(
        host=getattr(config, "REDIS_HOST", "localhost"),
        port=getattr(config, "REDIS_PORT", 6379),
        decode_responses=True,
        socket_connect_timeout=5,
    )
    try:
        await redis_client.ping()
        print("Successfully connected to Redis.")
    except Exception as e:
        print(f"Error connecting to Redis: {e}")
        

    yield

    print("Closing Redis connection...")
    if redis_client:
        await redis_client.close()
    print("Redis connection closed.")


# --- Redis Caching Middleware ---
class RedisCacheMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next):
        if request.method != "GET":
            return await call_next(request)

        path = request.url.path
        cache_expiry = CACHE_RULES.get(path)

        if cache_expiry is None:
            return await call_next(request)

        cache_key = f"cache:{str(request.url)}"
        
        try:
            cached = await redis_client.get(cache_key)
            if cached:
                return Response(content=cached, media_type="application/json")
        except Exception as e:
            print(f"Redis GET error: {e}. Skipping cache.")

        # Get fresh response
        response = await call_next(request)

        # Cache only successful JSON responses
        if response.status_code == 200 and "application/json" in response.headers.get("content-type", ""):
            body_bytes = [section async for section in response.body_iterator]
            raw_body = b"".join(body_bytes)

            try:
                await redis_client.set(cache_key, raw_body, ex=cache_expiry)
            except Exception as e:
                print(f"Redis SET error: {e}. Failed to cache response.")
            
            return Response(
                content=raw_body,
                status_code=response.status_code,
                headers=response.headers,
                media_type=response.media_type,
            )

        return response


# --- FastAPI App Initialization ---
app = FastAPI(
    title="BlazeDB API for STEEM Blockchain",
    description="API for STEEM Blockchain",
    version="0.0.1-alpha",
    contact={
        "name": "Blaze API",
        "url": "https://blazeapps.org",
        "email": "support@blazeapps.org",
    },
    license_info={"name": "MIT"},
    lifespan=lifespan, 
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

app.add_middleware(RedisCacheMiddleware)


app.include_router(blocks_router, prefix="/blocks", tags=["Blocks"])
#app.include_router(operations_router, prefix="/operations", tags=["Operations"])
#app.include_router(state_router, prefix="/state", tags=["State"])
#app.include_router(accounts_router, prefix="/accounts", tags=["Accounts"]) # Consolidated from two includes
#app.include_router(content_router, prefix="/content", tags=["Content"]) # Consolidated from two includes
#app.include_router(communities_router, prefix="/communities", tags=["Communities"])
#app.include_router(feeds_router, prefix="/feeds", tags=["Feeds"])
#app.include_router(market_router, prefix="/market", tags=["Market"])
#app.include_router(governance_router, prefix="/governance", tags=["Governance"])


# --- Utility Ping Endpoint ---
class PingResponse(BaseModel):
    response: str
    model_config = {"json_schema_extra": {"example": {"response": "pong"}}}


@app.get("/ping", summary="Health check", tags=["Utility"], response_model=PingResponse)
async def ping():
    """A simple endpoint to verify that the API is running."""
    return {"response": "pong"}

