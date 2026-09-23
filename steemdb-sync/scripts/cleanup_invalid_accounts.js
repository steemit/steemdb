// One-off cleanup: delete `account` documents whose _id is not a
// syntactically valid Steem account name. These stubs come from
// user-controlled custom_json payloads (e.g. id="follow" follower/following)
// that older sync builds upserted without validation — garbage like "\n",
// '"><i>', '"steemit-health"' or '#52 fyrstikken ...'.
//
// Valid Steem names: 3-16 chars, lowercase letters/digits plus '.' and '-',
// starting with a letter (same rule as IsValidAccountName in
// internal/processor/handlers/helpers.go).
//
// Usage (dry-run by default, prints counts and a sample only):
//   mongo "mongodb://<user>:<pass>@<host>:27017/<db>?authSource=admin" \
//     scripts/cleanup_invalid_accounts.js
// Actually delete:
//   mongo "mongodb://<user>:<pass>@<host>:27017/<db>?authSource=admin" \
//     --eval "var DRY_RUN=false" scripts/cleanup_invalid_accounts.js
(function () {
  var dryRun = typeof DRY_RUN === 'undefined' ? true : DRY_RUN;
  var validName = /^[a-z][a-z0-9.-]{2,15}$/;

  var invalidIds = [];
  db.account.find({}, { _id: 1 }).forEach(function (doc) {
    if (typeof doc._id !== 'string' || !validName.test(doc._id)) {
      invalidIds.push(doc._id);
    }
  });

  print('invalid account _id count: ' + invalidIds.length);
  print('sample: ' + JSON.stringify(invalidIds.slice(0, 20)));

  if (dryRun) {
    print('dry run — re-run with --eval "var DRY_RUN=false" to delete');
    return;
  }

  var removed = 0;
  for (var i = 0; i < invalidIds.length; i += 1000) {
    removed += db.account.deleteMany({ _id: { $in: invalidIds.slice(i, i + 1000) } }).deletedCount;
  }
  print('deleted: ' + removed);
})();
