import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import type { OperationData } from '../../types';

// Renders a one-line, link-rich summary of an operation's payload, e.g.
// "@alice → @bob · 1.500 STEEM" for a transfer. Unknown op types fall back
// to the involved-account list. All field access is defensive: malformed
// payloads degrade to the fallback instead of throwing.

function AccountLink({ name }: { name: string }) {
  return (
    <Link to={`/accounts/${name}`} className="text-primary hover:underline">
      @{name}
    </Link>
  );
}

function PostLink({ author, permlink }: { author: string; permlink: string }) {
  return (
    <Link to={`/posts/${author}/${permlink}`} className="text-primary hover:underline">
      {permlink}
    </Link>
  );
}

function str(v: unknown): string | undefined {
  return typeof v === 'string' && v !== '' ? v : undefined;
}

function num(v: unknown): number | undefined {
  return typeof v === 'number' && Number.isFinite(v) ? v : undefined;
}

function Field({ value }: { value: unknown }) {
  if (value === undefined || value === null || value === '') return null;
  return <span className="font-medium">{String(value)}</span>;
}

// Tries to surface account/post references from a custom_json payload.
// custom_json json is either [opName, payload] or a bare payload object.
function customJsonSummary(json: unknown): ReactNode {
  const raw = str(json);
  if (!raw) return null;
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }
  let payload: unknown = parsed;
  if (Array.isArray(parsed) && parsed.length === 2) {
    payload = parsed[1];
  }
  if (typeof payload !== 'object' || payload === null) return null;
  const p = payload as Record<string, unknown>;
  const follower = str(p.follower);
  const following = str(p.following);
  if (follower && following) {
    return (
      <>
        <AccountLink name={follower} /> → <AccountLink name={following} />
      </>
    );
  }
  const account = str(p.account);
  const author = str(p.author);
  const permlink = str(p.permlink);
  if (account && author && permlink) {
    return (
      <>
        <AccountLink name={account} /> on <PostLink author={author} permlink={permlink} />
      </>
    );
  }
  if (author && permlink) {
    return <PostLink author={author} permlink={permlink} />;
  }
  return null;
}

export function OpSummary({ op }: { op: OperationData }) {
  const d = op.data ?? {};
  const sep = <span className="text-muted-foreground"> · </span>;

  let body: ReactNode = null;
  switch (op.type) {
    case 'transfer':
    case 'transfer_to_vesting':
    case 'transfer_to_savings':
    case 'transfer_from_savings': {
      const from = str(d.from);
      const to = str(d.to);
      if (from && to) {
        body = (
          <>
            <AccountLink name={from} />
            <span className="text-muted-foreground"> → </span>
            <AccountLink name={to} />
            {sep}
            <Field value={d.amount} />
          </>
        );
      }
      break;
    }
    case 'vote': {
      const voter = str(d.voter);
      const author = str(d.author);
      const permlink = str(d.permlink);
      const weight = num(d.weight);
      if (voter && author && permlink) {
        body = (
          <>
            <AccountLink name={voter} />
            <span className="text-muted-foreground"> voted </span>
            <PostLink author={author} permlink={permlink} />
            {weight !== undefined && (
              <span className="text-muted-foreground"> ({weight / 100}%)</span>
            )}
          </>
        );
      }
      break;
    }
    case 'comment': {
      const author = str(d.author);
      const permlink = str(d.permlink);
      const parentAuthor = str(d.parent_author);
      if (author && permlink) {
        body = parentAuthor ? (
          <>
            <AccountLink name={author} />
            <span className="text-muted-foreground"> replied: </span>
            <PostLink author={author} permlink={permlink} />
          </>
        ) : (
          <>
            <AccountLink name={author} />
            <span className="text-muted-foreground"> posted: </span>
            <PostLink author={author} permlink={permlink} />
          </>
        );
      }
      break;
    }
    case 'custom_json': {
      const id = str(d.id);
      const detail = customJsonSummary(d.json);
      body = (
        <>
          {id && <Field value={id} />}
          {id && detail && sep}
          {detail}
        </>
      );
      break;
    }
    case 'account_witness_vote': {
      const account = str(d.account);
      const witness = str(d.witness);
      if (account && witness) {
        body = (
          <>
            <AccountLink name={account} />
            <span className="text-muted-foreground">
              {d.approve === false ? ' unvoted ' : ' approved '}
            </span>
            <AccountLink name={witness} />
          </>
        );
      }
      break;
    }
    case 'account_witness_proxy': {
      const account = str(d.account);
      const proxy = str(d.proxy);
      if (account) {
        body = proxy ? (
          <>
            <AccountLink name={account} />
            <span className="text-muted-foreground"> proxied to </span>
            <AccountLink name={proxy} />
          </>
        ) : (
          <>
            <AccountLink name={account} />
            <span className="text-muted-foreground"> removed proxy</span>
          </>
        );
      }
      break;
    }
    case 'claim_reward_balance': {
      const account = str(d.account);
      if (account) {
        body = (
          <>
            <AccountLink name={account} />
            <span className="text-muted-foreground"> claimed </span>
            <Field value={d.reward_steem} />
            {str(d.reward_sbd) && (
              <>
                {sep}
                <Field value={d.reward_sbd} />
              </>
            )}
          </>
        );
      }
      break;
    }
    case 'delegate_vesting_shares': {
      const delegator = str(d.delegator);
      const delegatee = str(d.delegatee);
      if (delegator && delegatee) {
        body = (
          <>
            <AccountLink name={delegator} />
            <span className="text-muted-foreground"> → </span>
            <AccountLink name={delegatee} />
            {sep}
            <Field value={d.vesting_shares} />
          </>
        );
      }
      break;
    }
    case 'withdraw_vesting': {
      const account = str(d.account);
      if (account) {
        body = (
          <>
            <AccountLink name={account} />
            {sep}
            <Field value={d.vesting_shares} />
          </>
        );
      }
      break;
    }
    case 'limit_order_create':
    case 'limit_order_create2': {
      const owner = str(d.owner);
      if (owner) {
        body = (
          <>
            <AccountLink name={owner} />
            <span className="text-muted-foreground"> sell </span>
            <Field value={d.amount_to_sell} />
            <span className="text-muted-foreground"> for </span>
            <Field value={d.min_to_receive} />
          </>
        );
      }
      break;
    }
    case 'convert': {
      const owner = str(d.owner);
      if (owner) {
        body = (
          <>
            <AccountLink name={owner} />
            {sep}
            <Field value={d.amount} />
          </>
        );
      }
      break;
    }
    case 'feed_publish': {
      const publisher = str(d.publisher);
      if (publisher) {
        body = <AccountLink name={publisher} />;
      }
      break;
    }
    case 'account_create':
    case 'account_create_with_delegation': {
      const creator = str(d.creator);
      const name = str(d.new_account_name);
      if (creator && name) {
        body = (
          <>
            <AccountLink name={creator} />
            <span className="text-muted-foreground"> created </span>
            <AccountLink name={name} />
          </>
        );
      }
      break;
    }
    default:
      break;
  }

  if (body) {
    return <span className="text-muted-foreground">{body}</span>;
  }

  // Fallback: involved accounts, as before.
  if (op.accounts && op.accounts.length > 0) {
    return (
      <span className="text-muted-foreground">
        {op.accounts.map((acc, i) => (
          <span key={acc}>
            {i > 0 && ', '}
            <AccountLink name={acc} />
          </span>
        ))}
      </span>
    );
  }
  return null;
}
