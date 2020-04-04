<div class="ui card">
  <div class="content">
    <?php if (isset($live[0]) && isset($live[0]) && isset($live[0]['profile']) && isset($live[0]['profile']['profile_image'])) { ?>
    <img class="right floated avatar image" src="<?= $this->escaper->escapeHtml($live[0]['profile']['profile_image']) ?>">
    <?php } ?>
    <div class="header">
      <span class="ui circular blue tiny label" style="margin-left: 0; vertical-align: top;">
        <?php echo $this->reputation::number($account->reputation) ?>
      </span>
      <a href="/@<?= $account->name ?>">
        <?= $account->name ?>
      </a>
    </div>
    <div class="meta">
      joined <?php echo $this->timeAgo::mongo($account->created); ?>
    </div>
    <?php if (isset($live[0]) && isset($live[0]) && isset($live[0]['profile']) && isset($live[0]['profile']['about'])) { ?>
    <div class="description">
      <?= $this->escaper->escapeHtml($live[0]['profile']['about']) ?>
    </div>
    <?php } ?>
    <?php if (isset($live[0]) && isset($live[0]) && isset($live[0]['profile']) && isset($live[0]['profile']['website'])) { ?>
      <div class="description">
        <br><i class="linkify icon"></i>
        <a rel="nofollow noopener" href="<?= $this->escaper->escapeHtml($live[0]['profile']['website']) ?>">
          <?= $this->escaper->escapeHtml($live[0]['profile']['website']) ?>
        </a>
      </div>
    <?php } ?>

  </div>
  <div class="extra content">
    <span class="right floated">
      <?php if (isset($live[0]) && isset($live[0]) && isset($live[0]['profile']) && isset($live[0]['profile']['location'])) { ?>
        <i class="marker icon"></i>
        <?= $this->escaper->escapeHtml($live[0]['profile']['location']) ?>
      <?php } ?>
    </span>
    <span>
      <i class="users icon"></i>
      <?= $this->length($account->followers) ?> Followers
    </span>
  </div>
</div>
