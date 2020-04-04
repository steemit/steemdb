<div class="ui three item inverted menu">
  <a href="/@<?= $account->name ?>/transfers" class="<?= ($this->router->getActionName() == 'transfers' ? 'active' : '') ?> blue item">Transfers</a>
  <a href="/@<?= $account->name ?>/powerup" class="<?= ($this->router->getActionName() == 'powerup' ? 'active' : '') ?> blue item">Power Ups</a>
  <a href="/@<?= $account->name ?>/powerdown" class="<?= ($this->router->getActionName() == 'powerdown' ? 'active' : '') ?> blue item">Power Downs</a>
</div>
<h3 class="ui header">
  Transfers
  <div class="sub header">
    Transfers to and from @<?= $account->name ?> of both STEEM and SBD.
  </div>
</h3>
<table class="ui table">
  <thead>
    <tr>
      <th>When</th>
      <th>From</th>
      <th>To</th>
      <th class="right aligned">Amount</th>
      <th class="left aligned">Type</th>
      <th>Memo</th>
    </tr>
  </thead>
  <tbody>
    <?php $v150959473867815167381iterated = false; ?><?php foreach ($transfers as $transfer) { ?><?php $v150959473867815167381iterated = true; ?>
    <tr>
      <td class="collapsing">
        <?php echo gmdate("Y-m-d H:i:s e", (string) $transfer->_ts / 1000) ?>
      </td>
      <td>
        <a href="/@<?= $transfer->from ?>">
          <?= $transfer->from ?>
        </a>
      </td>
      <td>
        <a href="/@<?= $transfer->to ?>">
          <?= $transfer->to ?>
        </a>
      </td>
      <td class="right aligned">
        <div class="ui small header">
          <?php echo number_format($transfer->amount, 3, ".", ",") ?>
        </div>
      </td>
      <td class="left aligned">
        <?= $transfer->type ?>
      </td>
      <td class="collapsing">
        <?php if ($transfer->memo) { ?>
        <div class="ui icon mini button" data-popup data-content="<?= $transfer->memo ?>">
          <i class="sticky note outline icon"></i>
        </div>
        <?php } ?>
      </td>
    </tr>
  </tbody>
  <?php } if (!$v150959473867815167381iterated) { ?>
  <tbody>
    <tr>
      <td colspan="10">
        <div class="ui header">
          No powerdown transfers found
        </div>
      </td>
    </tr>
  </tbody>
  <?php } ?>
</table>
<div class="ui stackable grid">
  <div class="row">
    <?php if (!isset($hide_count)) { ?>
    <div class="seven wide column mobile hidden">
      Page <?= $page ?> of <?= $pages ?>
    </div>
    <?php } ?>
    <div class="right aligned nine wide column">
      <div class="ui small pagination menu">
        <?php if ($page <= 1) { ?>
        <a class="paginate_button item previous disabled" href="#" tabindex="0">
        <?php } else { ?>
        <a class="paginate_button item previous" href="?page=<?= $page - 1 ?>" tabindex="0">
        <?php } ?>
          Previous
        </a>
        <?php if ($page > 3) { ?>
        <a class="paginate_button item" href="?page=1" tabindex="0">
          1
        </a>
        <?php } ?>
        <?php if ($page - 2 > 0) { ?>
        <a class="paginate_button mobile hidden item" href="?page=<?= $page - 2 ?>" tabindex="0">
          <?= $page - 2 ?>
        </a>
        <?php } ?>
        <?php if ($page - 1 > 0) { ?>
        <a class="paginate_button item" href="?page=<?= $page - 1 ?>" tabindex="0">
          <?= $page - 1 ?>
        </a>
        <?php } ?>
        <a class="paginate_button item active" href="?page=<?= $page ?>" tabindex="0">
          <?= $page ?>
        </a>
        <?php if ($page + 1 <= $pages) { ?>
        <a class="paginate_button item" href="?page=<?= $page + 1 ?>" tabindex="0">
          <?= $page + 1 ?>
        </a>
        <?php } ?>
        <?php if ($page + 2 <= $pages) { ?>
        <a class="paginate_button mobile hidden item" href="?page=<?= $page + 2 ?>" tabindex="0">
          <?= $page + 2 ?>
        </a>
        <?php } ?>
        <?php if ($page + 3 <= $pages) { ?>
        <a class="paginate_button mobile hidden item" href="?page=<?= $page + 3 ?>" tabindex="0">
          <?= $page + 3 ?>
        </a>
        <?php } ?>
        <?php if ($page + 1 <= $pages) { ?>
        <a class="paginate_button item next" id="example_next" href="?page=<?= $page + 1 ?>" tabindex="0">
          Next
        </a>
        <?php } ?>
      </div>
    </div>
  </div>
</div>

