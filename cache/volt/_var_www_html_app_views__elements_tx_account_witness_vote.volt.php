<span class="ui left labeled button" tabindex="0">
  <?php if ($item[1]['op'][1]['approve']) { ?>
  <span class="ui purple right pointing label">
    Approval of
  </span>
  <?php } else { ?>
  <span class="ui orange right pointing label">
    Unapproved
  </span>
  <?php } ?>
  <a href="/@<?= $item[1]['op'][1]['witness'] ?>" class="ui button">
    <?= $item[1]['op'][1]['witness'] ?>
  </a>
</span>
from
<a href="/@<?= $item[1]['op'][1]['account'] ?>">
  <?= $item[1]['op'][1]['account'] ?>
</a>
