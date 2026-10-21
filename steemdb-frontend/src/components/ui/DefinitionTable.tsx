import { Link } from 'react-router-dom';

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
export function DefinitionTable({ data, extra }: { data: Record<string, unknown>; extra?: [string, unknown][] }) {
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
