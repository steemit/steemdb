<h3 class="ui dividing header">
  Account Raw Data
  <div class="sub header">
    Snapshot of blockchain information cached <?php echo $this->timeAgo::mongo($account->scanned); ?>
  </div>
</h3>
<?php $this->partial('_elements/definition_table', ['data' => $account]); ?>
