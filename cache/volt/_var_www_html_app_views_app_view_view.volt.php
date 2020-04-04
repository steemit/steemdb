<div class="ui stackable grid">
  <div class="row">
    <div class="twelve wide column">
      <h3 class="ui header">
        Platform Leaderboard (30-Days)
        <div class="sub header">
          The most rewarded authors using the <strong><?= $app ?></strong> platform to create content.
        </div>
      </h3>
    </div>
  </div>
</div>

<table class="ui stackable definition table">
  <thead>
    <tr>
      <th class='collapsing'></th>
      <th>Date</th>
      <th>Posts</th>
      <th>SBD</th>
      <th>VESTS</th>
      <th>STEEM</th>
    </tr>
  </thead>
  <tbody>
  <?php $v170509095852014938811iterated = false; ?><?php $v170509095852014938811iterator = $leaderboard; $v170509095852014938811incr = 0; $v170509095852014938811loop = new stdClass(); $v170509095852014938811loop->self = &$v170509095852014938811loop; $v170509095852014938811loop->length = count($v170509095852014938811iterator); $v170509095852014938811loop->index = 1; $v170509095852014938811loop->index0 = 1; $v170509095852014938811loop->revindex = $v170509095852014938811loop->length; $v170509095852014938811loop->revindex0 = $v170509095852014938811loop->length - 1; ?><?php foreach ($v170509095852014938811iterator as $item) { ?><?php $v170509095852014938811loop->first = ($v170509095852014938811incr == 0); $v170509095852014938811loop->index = $v170509095852014938811incr + 1; $v170509095852014938811loop->index0 = $v170509095852014938811incr; $v170509095852014938811loop->revindex = $v170509095852014938811loop->length - $v170509095852014938811incr; $v170509095852014938811loop->revindex0 = $v170509095852014938811loop->length - ($v170509095852014938811incr + 1); $v170509095852014938811loop->last = ($v170509095852014938811incr == ($v170509095852014938811loop->length - 1)); ?><?php $v170509095852014938811iterated = true; ?>
  <tr>
    <td class='right aligned'>
      <?= $v170509095852014938811loop->index ?>
    </td>
    <td class="three wide">
      <div class="ui small header">
        <a href="/@<?= $item->_id['account'] ?>">
          <?= $item->_id['account'] ?>
        </a>
      </div>
    </td>
    <td class='right aligned'>
      <a href="/@<?= $item->_id['account'] ?>/posts">
        <?= $item->count ?>
      </a>
    </td>
    <td class='right aligned'>
      <div class="ui small header">
        <?= $item->sbd ?> SBD
      </div>
    </td>
    <td class='right aligned'>
      <div class="ui small header">
        <?php echo $this->convert->vest2sp($item->vests, null) ?> SP
        <div class="sub header">
          <?= $item->vests ?> VESTS
        </div>
      </div>
    </td>
    <td class='right aligned'>
      <?= $item->steem ?> STEEM
    </td>
  </tr>
  <?php $v170509095852014938811incr++; } if (!$v170509095852014938811iterated) { ?>
  <tr>
    <td>
      No rewards found.
    </td>
  </tr>
  <?php } ?>
  </tbody>
</table>
