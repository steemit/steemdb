<table class="ui unstackable table" id="table-votes">
  <thead>
    <tr>
      <th class="mobile hidden">%</th>
      <th>Voter</th>
      <th class="mobile hidden">Time</th>
      <th class="mobile hidden right aligned">Weight</th>
      <th class="right aligned">Reward Shares</th>
    </tr>
  </thead>
  <tbody>
    <?php foreach ($votes as $voter) { ?>
    <tr>
      <td class="mobile hidden">
        <?= $voter->percent / 100 ?>%
      </td>
      <td>
        <a href="/@<?= $voter->voter ?>">
          <?= $voter->voter ?>
        </a>
      </td>
      <td class="mobile hidden">
        <?= date('Y-m-d H:i:s', $voter->time / 1000) ?>
      </td>
      <td class="mobile hidden right aligned">
        <div style="display: none"><?php echo number_format($voter->weight, 0, "", "") ?></div>
<div class="ui <?php echo $this->largeNumber::color($voter->weight / 1000000)?> label" data-popup data-content="<?php echo number_format($voter->weight, 3, ".", ",") ?> VESTS" data-variation="inverted" data-position="left center">
  <?php echo $this->largeNumber::format($voter->weight); ?>
</div>

      </td>
      <td class="right aligned">
        <div style="display: none"><?= $voter->rshares ?></div>
<div class="ui <?php echo $this->largeNumber::color($voter->rshares / 1000)?> label" data-popup data-content="<?php echo number_format($voter->rshares, 3, ".", ",") ?> Reward Shares" data-variation="inverted" data-position="left center">
  <?php echo $this->largeNumber::format($voter->rshares, 'RS'); ?>
</div>

      </td>
    </tr>
    <?php } ?>
  </tbody>
</table>
