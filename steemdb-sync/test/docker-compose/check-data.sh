#!/bin/bash
# Validate MongoDB data integrity for docker-compose cold_ingest tests.
# Checks:
# - expected max block derivation (STOP_REPLAY_AT_BLOCK > 0 else meta.sync_state.max_block)
# - missing blocks in [1..expected_max_block]
# - orphan operations (operations referencing missing blocks)
# - blocks-with-zero-ops count (sanity)
#
# Exit code:
# - 0: OK
# - 1: Any missing/inconsistency detected

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

fail() {
  echo -e "${RED}ERROR:${NC} $*" >&2
  exit 1
}

echo -e "${GREEN}=== Checking Cold Ingest Mongo Data (blocks + operations) ===${NC}\n"

# Load env if present
if [ -f .env ]; then
  # shellcheck disable=SC1091
  source .env
fi

MONGO_USERNAME="${MONGO_USERNAME:-admin}"
MONGO_PASSWORD="${MONGO_PASSWORD:-123456}"
MONGO_DATABASE="${MONGO_DATABASE:-steemdb_test}"

STOP_REPLAY_AT_BLOCK="${STOP_REPLAY_AT_BLOCK:-0}"

echo -e "${BLUE}Preflight: docker-compose + Mongo...${NC}"
if ! docker-compose ps | grep -q "Up"; then
  fail "No services are running. Start with: ./start.sh"
fi

if ! docker-compose exec -T mongo mongo --quiet --eval "db.adminCommand('ping')" \
  --username "${MONGO_USERNAME}" --password "${MONGO_PASSWORD}" --authenticationDatabase admin >/dev/null 2>&1; then
  fail "MongoDB is not responding"
fi
echo -e "${GREEN}✓ MongoDB is healthy${NC}\n"

mongo_eval() {
  local js="$1"
  docker-compose exec -T mongo mongo --quiet "${MONGO_DATABASE}" \
    --username "${MONGO_USERNAME}" --password "${MONGO_PASSWORD}" --authenticationDatabase admin \
    --eval "${js}"
}

echo -e "${BLUE}Deriving expected_max_block...${NC}"
EXPECTED_MAX=""
if [[ "${STOP_REPLAY_AT_BLOCK}" =~ ^[0-9]+$ ]] && [ "${STOP_REPLAY_AT_BLOCK}" -gt 0 ]; then
  EXPECTED_MAX="${STOP_REPLAY_AT_BLOCK}"
  echo -e "${GREEN}✓ expected_max_block=${EXPECTED_MAX}${NC} (from STOP_REPLAY_AT_BLOCK)\n"
else
  EXPECTED_MAX="$(mongo_eval 'var m=db.meta.findOne({_id:"sync_state"}); if(!m||!m.max_block){print("")} else {print(m.max_block)}' | tr -d '\r\n')"
  if [ -z "${EXPECTED_MAX}" ]; then
    fail "Cannot infer expected_max_block: STOP_REPLAY_AT_BLOCK=0 and meta.sync_state.max_block not found"
  fi
  echo -e "${GREEN}✓ expected_max_block=${EXPECTED_MAX}${NC} (from meta.sync_state.max_block)\n"
fi

echo -e "${BLUE}Querying blocks/operations summary...${NC}"
SUMMARY="$(
  mongo_eval "
    var expected = Number(${EXPECTED_MAX});
    if (!expected || expected < 1) { print('error=invalid_expected'); quit(); }

    var blocksCount = db.blocks.countDocuments({_id: {\$gte: 1, \$lte: expected}});
    var blocksMinDoc = db.blocks.find({_id: {\$gte: 1, \$lte: expected}}, {_id: 1}).sort({_id: 1}).limit(1).toArray();
    var blocksMaxDoc = db.blocks.find({_id: {\$gte: 1, \$lte: expected}}, {_id: 1}).sort({_id: -1}).limit(1).toArray();
    var blocksMin = (blocksMinDoc.length ? blocksMinDoc[0]._id : null);
    var blocksMax = (blocksMaxDoc.length ? blocksMaxDoc[0]._id : null);

    // Missing blocks scan: walk sorted _id cursor, detect gaps.
    var missing = [];
    var prev = 0;
    var cur = db.blocks.find({_id: {\$gte: 1, \$lte: expected}}, {_id: 1}).sort({_id: 1});
    while (cur.hasNext()) {
      var d = cur.next();
      var id = Number(d._id);
      if (id > prev + 1) {
        // add range (prev+1 .. id-1)
        for (var x = prev + 1; x <= id - 1; x++) missing.push(x);
      }
      prev = id;
    }
    if (prev < expected) {
      for (var x2 = prev + 1; x2 <= expected; x2++) missing.push(x2);
    }

    function ranges(nums) {
      if (!nums || nums.length === 0) return '';
      nums.sort(function(a,b){return a-b});
      var out = [];
      var s = nums[0], e = nums[0];
      for (var i=1;i<nums.length;i++) {
        var n = nums[i];
        if (n === e + 1) { e = n; continue; }
        out.push(s === e ? String(s) : (String(s) + '-' + String(e)));
        s = e = n;
      }
      out.push(s === e ? String(s) : (String(s) + '-' + String(e)));
      return out.join(',');
    }

    var opsTotal = db.operations.countDocuments({});

    // Orphan operations (ops.block_num has no matching blocks._id)
    // NOTE: This may be expensive on huge datasets; intended for docker-compose tests / limited replays.
    var orphan = db.operations.aggregate([
      { \$lookup: { from: 'blocks', localField: 'block_num', foreignField: '_id', as: 'b' } },
      { \$match: { b: { \$eq: [] } } },
      { \$group: { _id: null, count: { \$sum: 1 }, sample: { \$push: '\$_id' } } },
      { \$project: { _id: 0, count: 1, sample: { \$slice: [ '\$sample', 5 ] } } }
    ]).toArray();
    var orphanCount = (orphan.length ? orphan[0].count : 0);
    var orphanSample = (orphan.length ? orphan[0].sample : []);

    // Blocks-with-ops approximation in range
    var opsBlocksDistinct = db.operations.distinct('block_num', { block_num: { \$gte: 1, \$lte: expected } });
    var blocksWithOpsCount = opsBlocksDistinct.length;
    var blocksZeroOps = blocksCount - blocksWithOpsCount;
    if (blocksZeroOps < 0) blocksZeroOps = 0;

    // Tail histogram (last 20 blocks within [1..expected])
    var tailN = 20;
    var tailStart = expected - tailN + 1;
    if (tailStart < 1) tailStart = 1;
    var hist = [];
    for (var bn = tailStart; bn <= expected; bn++) {
      var c = db.operations.countDocuments({ block_num: bn });
      hist.push({ block_num: bn, ops: c });
    }

    print('expected_max_block=' + expected);
    print('blocks_count_in_range=' + blocksCount);
    print('blocks_min_in_range=' + (blocksMin === null ? '' : blocksMin));
    print('blocks_max_in_range=' + (blocksMax === null ? '' : blocksMax));
    print('missing_count=' + missing.length);
    print('missing_sample=' + missing.slice(0,20).join(','));
    print('missing_ranges=' + ranges(missing));
    print('ops_total=' + opsTotal);
    print('orphan_ops_count=' + orphanCount);
    print('orphan_ops_sample=' + orphanSample.join(','));
    print('blocks_with_ops_in_range=' + blocksWithOpsCount);
    print('blocks_zero_ops_in_range=' + blocksZeroOps);
    print('tail_histogram=' + JSON.stringify(hist));
  "
)"

echo "$SUMMARY" | sed 's/\r$//'
echo ""

get_kv() {
  local key="$1"
  echo "$SUMMARY" | sed 's/\r$//' | awk -F= -v k="$key" '$1==k {sub($1"=",""); print; exit}'
}

missing_count="$(get_kv missing_count)"
orphan_ops_count="$(get_kv orphan_ops_count)"
blocks_count_in_range="$(get_kv blocks_count_in_range)"
blocks_max_in_range="$(get_kv blocks_max_in_range)"

if [ -z "${blocks_count_in_range}" ] || [ "${blocks_count_in_range}" -eq 0 ]; then
  fail "No blocks found in range [1..${EXPECTED_MAX}]. Did replay run?"
fi

echo -e "${BLUE}Interpretation:${NC}"
echo -e "  expected_max_block: ${EXPECTED_MAX}"
echo -e "  blocks_max_in_range: ${blocks_max_in_range:-<none>}"
echo -e "  missing_count:       ${missing_count:-0}"
echo -e "  orphan_ops_count:    ${orphan_ops_count:-0}"
echo ""

issues=0
if [ "${missing_count:-0}" -ne 0 ]; then
  echo -e "${RED}✗ Missing blocks detected${NC}"
  issues=1
else
  echo -e "${GREEN}✓ No missing blocks in expected range${NC}"
fi

if [ "${orphan_ops_count:-0}" -ne 0 ]; then
  echo -e "${RED}✗ Orphan operations detected (ops referencing missing blocks)${NC}"
  issues=1
else
  echo -e "${GREEN}✓ No orphan operations${NC}"
fi

echo ""
if [ "$issues" -ne 0 ]; then
  echo -e "${RED}=== CHECK FAILED ===${NC}"
  exit 1
fi

echo -e "${GREEN}=== CHECK PASSED ===${NC}"
exit 0

