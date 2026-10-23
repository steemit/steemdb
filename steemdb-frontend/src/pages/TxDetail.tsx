import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { Card, CardContent } from '../components/ui/Card';
import { DefinitionTable } from '../components/ui/DefinitionTable';
import { getTransaction } from '../lib/api';
import type { ApiErrorBody, ApiResponse, TransactionDetail } from '../types';
import { formatNumber, opDisplayName } from '../lib/utils';

// apiErrorCode extracts the backend's numeric error code from an ApiResponse
// error envelope (see lib/api.ts: failed requests keep the backend body).
function apiErrorCode(error: ApiResponse<TransactionDetail>['error'] | undefined): number | undefined {
  return typeof error === 'object' && error !== null ? (error as ApiErrorBody).code : undefined;
}

// TxOperationRow renders one operation of the transaction: a header with the
// operation name and a collapsible definition table of the payload.
function TxOperationRow({ opType, opValue }: { opType: string; opValue: Record<string, unknown> }) {
  const [open, setOpen] = useState(false);
  const hasValue = opValue && Object.keys(opValue).length > 0;

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
          <span className="font-medium">{opDisplayName(opType)}</span>
        </div>
      </button>
      {open && (
        <div className="border-t bg-muted/20 px-3 py-3">
          {hasValue ? (
            <DefinitionTable data={opValue} />
          ) : (
            <div className="py-2 text-sm text-muted-foreground">No operation data available.</div>
          )}
        </div>
      )}
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

type TabId = 'operations' | 'transaction' | 'json';

export function TxDetailPage() {
  const { id } = useParams<{ id: string }>();
  const txId = id ?? '';
  const isValidId = /^[0-9a-fA-F]{40}$/.test(txId);

  const { data, isLoading, error } = useQuery({
    queryKey: ['tx', txId],
    queryFn: () => getTransaction(txId),
    enabled: isValidId,
    // While the transaction's block is still inside the reversible window
    // the API answers 425; poll until the chain finalizes it and the page
    // fills in on its own (Steem irreversibility lags the head by well
    // under a minute).
    refetchInterval: (query) =>
      apiErrorCode(query.state.data?.error) === 425 ? 5000 : false,
  });

  const tx: TransactionDetail | undefined = data?.data;
  const operations = tx?.operations ?? [];

  const [activeTab, setActiveTab] = useState<TabId | null>(null);

  if (!isValidId) {
    return (
      <div className="space-y-6">
        <Card>
          <CardContent className="py-12">
            <div className="text-center text-muted-foreground">
              Invalid transaction ID. A transaction ID is 40 hexadecimal characters.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading transaction...</div>
        </div>
      </div>
    );
  }

  const notYetIrreversible = apiErrorCode(data?.error) === 425;

  if (error || !data?.success || !tx) {
    return (
      <div className="space-y-6">
        <Card>
          <CardContent className="py-12">
            {notYetIrreversible ? (
              <div className="space-y-2">
                <div className="text-center font-medium">Transaction not yet irreversible</div>
                <div className="text-center text-muted-foreground">
                  This transaction has landed on chain, but its block is still inside the
                  reversible window. SteemDB only displays irreversible data — it becomes
                  viewable once the chain finalizes the block, usually within a minute.
                </div>
              </div>
            ) : (
              <div className="text-center text-muted-foreground">
                Transaction not found or failed to load.
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    );
  }

  const defaultTab: TabId = operations.length > 0 ? 'operations' : 'transaction';
  const currentTab: TabId = activeTab ?? defaultTab;

  const tabs: { id: TabId; label: string; enabled: boolean }[] = [
    { id: 'operations', label: 'Operations', enabled: operations.length > 0 },
    { id: 'transaction', label: 'Transaction Data', enabled: true },
    { id: 'json', label: 'JSON', enabled: true },
  ];

  const txDataView: Record<string, unknown> = {
    transaction_id: txId,
    block_num: tx.block_num,
    transaction_num: tx.transaction_num,
    ref_block_num: tx.ref_block_num,
    ref_block_prefix: tx.ref_block_prefix,
    expiration: tx.expiration,
    expired: tx.expired,
    extensions: tx.extensions,
    signatures: tx.signatures,
  };

  return (
    <div className="space-y-6">
      {/* Header: transaction ID and containing block */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Transaction</h1>
        <p className="mt-1 break-all font-mono text-sm text-muted-foreground">{txId}</p>
        {tx.block_num > 0 && (
          <p className="mt-1 text-muted-foreground">
            ↳ Included in{' '}
            <Link to={`/blocks/${tx.block_num}`} className="text-primary hover:underline">
              Block #{formatNumber(tx.block_num)}
            </Link>
          </p>
        )}
      </div>

      {/* Tabbed data panels */}
      <div className="rounded-lg border">
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
        <div className="min-w-0 overflow-hidden p-4">
          {currentTab === 'operations' && (
            <div className="rounded-md border divide-y">
              {operations.map(([opType, opValue], index) => (
                <TxOperationRow key={`${opType}-${index}`} opType={opType} opValue={opValue} />
              ))}
            </div>
          )}
          {currentTab === 'transaction' && (
            <div className="rounded-md border">
              <DefinitionTable data={txDataView} />
            </div>
          )}
          {currentTab === 'json' && <JsonTab data={tx} />}
        </div>
      </div>
    </div>
  );
}
