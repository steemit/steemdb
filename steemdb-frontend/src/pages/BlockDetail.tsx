import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, ArrowRight, ChevronDown, ChevronRight } from 'lucide-react';
import { Card, CardContent } from '../components/ui/Card';
import { getBlock, getBlockVirtualOps } from '../lib/api';
import type { Block, BlockOperation, BlockTransaction, VirtualOperation } from '../types';
import { formatNumber, formatTimeAgo } from '../lib/utils';

// Pretty operation names, ported from the legacy app/helpers/OpName.php map.
// Types without an entry fall back to the raw op_type string.
const OP_NAMES: Record<string, string> = {
  account_create: 'Account Create',
  account_update: 'Account Update',
  account_witness_proxy: 'Witness Proxy',
  account_witness_vote: 'Witness Vote',
  author_reward: 'Author Reward',
  cancel_transfer_from_savings: 'Cancel Savings Withdrawal',
  comment: 'Post',
  comment_reward: 'Post Reward',
  convert: 'Convert',
  curate_reward: 'Curate Reward',
  curation_reward: 'Curation Reward',
  delete_comment: 'Post Delete',
  feed_publish: 'Feed Publish',
  fill_order: 'Fill Order',
  fill_vesting_withdraw: 'Power Down',
  interest: 'SBD Interest',
  limit_order_create: 'Limit Order Create',
  limit_order_cancel: 'Limit Order Cancel',
  pow: 'Mining',
  pow2: 'Mining',
  transfer: 'Transfer',
  transfer_to_savings: 'Transfer to Savings',
  transfer_from_savings: 'Transfer from Savings',
  transfer_to_vesting: 'Power Up',
  vote: 'Vote',
  witness_update: 'Witness Update',
};

function opDisplayName(opType: string): string {
  return OP_NAMES[opType] ?? opType;
}

// Keys whose values are Steem account names, rendered as links.
const ACCOUNT_KEYS = new Set([
  'account',
  'approver',
  'author',
  'creator',
  'current_owner',
  'delegator',
  'delegatee',
  'from',
  'new_owner',
  'owner',
  'publisher',
  'recoverer',
  'requester',
  'to',
  'voter',
  'witness',
]);

function isPrimitive(value: unknown): value is string | number | boolean {
  return typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean';
}

function ValueCell({ value, accountKey }: { value: unknown; accountKey?: string }) {
  if (value === null || value === undefined || value === '') {
    return <span className="text-muted-foreground">—</span>;
  }
  if (isPrimitive(value)) {
    if (accountKey && ACCOUNT_KEYS.has(accountKey) && typeof value === 'string') {
      return (
        <Link to={`/accounts/${value}`} className="text-primary hover:underline">
          {value}
        </Link>
      );
    }
    return <span className="break-all">{String(value)}</span>;
  }
  // Arrays and objects render as pretty JSON, like the legacy definition table.
  return (
    <pre className="max-h-64 overflow-auto rounded bg-muted p-2 text-xs leading-relaxed">
      {JSON.stringify(value, null, 2)}
    </pre>
  );
}

// DefinitionTable mirrors the legacy _elements/definition_table view: a
// two-column key/value table with the key column visually distinct.
function DefinitionTable({ data, extra }: { data: Record<string, unknown>; extra?: [string, unknown][] }) {
  const entries: [string, unknown][] = [...Object.entries(data), ...(extra ?? [])];
  if (entries.length === 0) {
    return <div className="py-4 text-center text-sm text-muted-foreground">No data</div>;
  }
  return (
    <table className="w-full table-fixed text-sm">
      <tbody>
        {entries.map(([key, value]) => (
          <tr key={key} className="border-b last:border-b-0">
            <td className="w-1/3 bg-muted/50 px-3 py-2 align-top font-medium text-muted-foreground">
              <small>{key}</small>
            </td>
            <td className="px-3 py-2 align-top">
              <ValueCell value={value} accountKey={key} />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// OperationRow renders one operation of the Operations tab: a header with the
// operation name / relative time / block link, and a collapsible definition
// table of the operation payload plus the containing txid.
function OperationRow({ op, blockNum, timestamp }: { op: BlockOperation; blockNum: number; timestamp: string }) {
  const [open, setOpen] = useState(false);
  const hasValue = op.op_value && Object.keys(op.op_value).length > 0;

  return (
    <div className="border-b last:border-b-0">
      <button
        className="flex w-full items-center justify-between gap-4 px-3 py-3 text-left hover:bg-accent/50"
        onClick={() => setOpen(!open)}
      >
        <div className="flex items-center gap-2">
          {open ? (
            <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
          )}
          <span className="font-medium">{opDisplayName(op.op_type)}</span>
        </div>
        <div className="shrink-0 text-right text-xs text-muted-foreground">
          {formatTimeAgo(timestamp)}
          <br />
          <span className="text-[11px]">Block #{formatNumber(blockNum)}</span>
        </div>
      </button>
      {open && (
        <div className="border-t bg-muted/20 px-3 py-3">
          {hasValue ? (
            <DefinitionTable data={op.op_value} extra={op.trx_id ? [['txid', op.trx_id]] : []} />
          ) : (
            <div className="py-2 text-sm text-muted-foreground">No operation data available.</div>
          )}
        </div>
      )}
    </div>
  );
}

function TransactionsTab({ transactions }: { transactions: BlockTransaction[] }) {
  return (
    <div className="space-y-4">
      {transactions.map((tx, index) => (
        <div key={tx.transaction_id || index} className="rounded-md border">
          <div className="border-b bg-muted/50 px-3 py-2 text-sm">
            <span className="font-medium">Transaction #{index}</span>
            <span className="ml-2 break-all font-mono text-xs text-muted-foreground">{tx.transaction_id}</span>
          </div>
          <DefinitionTable
            data={{
              ref_block_num: tx.ref_block_num,
              ref_block_prefix: tx.ref_block_prefix,
              expiration: tx.expiration,
              operations: tx.operations?.map((op) => ({ [op.op_type]: op.op_value })),
              extensions: tx.extensions,
              signatures: tx.signatures,
            }}
          />
        </div>
      ))}
    </div>
  );
}

function VirtualOpsTab({ ops }: { ops: VirtualOperation[] }) {
  return (
    <div className="rounded-md border divide-y">
      {ops.map((op, index) => (
        <div key={`${op.virtual_op}-${op.op_type}-${index}`}>
          <div className="flex flex-wrap items-baseline justify-between gap-2 px-3 pt-3">
            <div className="flex items-baseline gap-2">
              <span className="font-mono text-xs text-muted-foreground">#{op.virtual_op}</span>
              <span className="font-medium">{opDisplayName(op.op_type)}</span>
            </div>
            <span className="text-xs text-muted-foreground">{formatTimeAgo(op.timestamp)}</span>
          </div>
          <div className="px-3 pb-3 pt-1">
            <DefinitionTable data={op.op_value ?? {}} />
          </div>
        </div>
      ))}
    </div>
  );
}

function JsonTab({ data }: { data: unknown }) {
  return (
    <div className="min-w-0">
      <pre className="max-h-[32rem] max-w-full overflow-auto rounded-md bg-muted p-4 text-xs leading-relaxed">
        {JSON.stringify(data, null, 2)}
      </pre>
    </div>
  );
}

type TabId = 'operations' | 'transactions' | 'block' | 'virtual_ops' | 'json';

export function BlockDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const blockNumber = Number(id);

  const { data, isLoading, error } = useQuery({
    queryKey: ['block', blockNumber],
    queryFn: () => getBlock(blockNumber),
    enabled: Number.isFinite(blockNumber) && blockNumber > 0,
  });

  // Virtual ops are fetched from the RPC on every view, like the legacy
  // getVirtualOpsInBlock call; the tab is only offered when any exist.
  const { data: virtualOpsData } = useQuery({
    queryKey: ['block-virtual-ops', blockNumber],
    queryFn: () => getBlockVirtualOps(blockNumber),
    enabled: Number.isFinite(blockNumber) && blockNumber > 0,
  });

  const block: Block | undefined = data?.data;
  const transactions = block?.transactions ?? [];
  const virtualOps = virtualOpsData?.data ?? [];

  const [activeTab, setActiveTab] = useState<TabId | null>(null);

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading block details...</div>
        </div>
      </div>
    );
  }

  if (error || !data?.success || !block) {
    return (
      <div className="space-y-6">
        <Card>
          <CardContent className="py-12">
            <div className="text-center text-muted-foreground">
              Block not found or failed to load.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  // Default tab mirrors the legacy behaviour: Operations when the block has
  // transactions, otherwise Block Data. Virtual Ops only when present.
  const defaultTab: TabId = transactions.length > 0 ? 'operations' : 'block';
  const currentTab: TabId = activeTab ?? defaultTab;

  const tabs: { id: TabId; label: string; enabled: boolean }[] = [
    { id: 'operations', label: 'Operations', enabled: transactions.length > 0 },
    { id: 'transactions', label: 'Transactions', enabled: transactions.length > 0 },
    { id: 'block', label: 'Block Data', enabled: true },
    { id: 'virtual_ops', label: 'Virtual Ops', enabled: virtualOps.length > 0 },
    { id: 'json', label: 'JSON', enabled: true },
  ];

  const blockDataView: Record<string, unknown> = {
    block_num: block.block_num,
    block_id: block.block_id,
    previous: block.previous,
    timestamp: block.timestamp,
    witness: block.witness,
    transaction_merkle_root: block.transaction_merkle_root,
    witness_signature: block.witness_signature,
    extensions: block.extensions,
    transaction_count: block.transaction_count,
    operation_count: block.operation_count,
    ...(block.transaction_ids ? { transaction_ids: block.transaction_ids } : {}),
  };

  return (
    <div className="space-y-6">
      {/* Header: block number, witness and timestamp */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Block {formatNumber(block.block_num)}</h1>
        <p className="mt-1 text-muted-foreground">
          ↳ Witnessed by{' '}
          <Link to={`/accounts/${block.witness}`} className="text-primary hover:underline">
            {block.witness}
          </Link>{' '}
          on {block.timestamp}
        </p>
      </div>

      {/* Previous / next block navigation */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-y py-3">
        <button
          onClick={() => navigate(`/blocks/${block.block_num - 1}`)}
          className="inline-flex items-center gap-2 rounded-md border px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Block #{formatNumber(block.block_num - 1)}
        </button>
        <button
          onClick={() => navigate(`/blocks/${block.block_num + 1}`)}
          className="inline-flex items-center gap-2 rounded-md border px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          Block #{formatNumber(block.block_num + 1)}
          <ArrowRight className="h-4 w-4" />
        </button>
      </div>

      {/* Tabbed data panels */}
      <div className="rounded-lg border">
        {/* Tabs wrap on narrow screens instead of forcing the page wider
            (nowrap buttons would set a ~530px min-content width). */}
        <div className="flex flex-wrap gap-1 border-b px-2 pt-2">
          {tabs
            .filter((tab) => tab.enabled)
            .map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`whitespace-nowrap rounded-t-md border-b-2 px-4 py-2 text-sm font-medium transition-colors ${
                  currentTab === tab.id
                    ? 'border-primary text-foreground'
                    : 'border-transparent text-muted-foreground hover:text-foreground'
                }`}
              >
                {tab.label}
              </button>
            ))}
        </div>
        <div className="p-4">
          {currentTab === 'operations' && (
            <div className="rounded-md border divide-y">
              {transactions.flatMap((tx) =>
                (tx.operations ?? []).map((op) => (
                  <OperationRow
                    key={op.id || `${op.trx_index}-${op.op_index}`}
                    op={op}
                    blockNum={block.block_num}
                    timestamp={block.timestamp}
                  />
                )),
              )}
            </div>
          )}
          {currentTab === 'transactions' && transactions.length > 0 && (
            <TransactionsTab transactions={transactions} />
          )}
          {currentTab === 'block' && (
            <div className="rounded-md border">
              <DefinitionTable data={blockDataView} />
            </div>
          )}
          {currentTab === 'virtual_ops' && virtualOps.length > 0 && <VirtualOpsTab ops={virtualOps} />}
          {currentTab === 'json' && <JsonTab data={block} />}
        </div>
      </div>
    </div>
  );
}
