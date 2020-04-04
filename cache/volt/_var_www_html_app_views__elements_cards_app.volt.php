<div class="ui card">
  <div class="content">
    
    <div class="header">
      <a href="/app/<?= $app ?>">
        <?= $app ?>
      </a>
    </div>
    <?php if ($meta && $meta['created']) { ?>
    <div class="meta">
      created <?php echo $this->timeAgo::mongo($meta[$app]['created']); ?>
    </div>
    <?php } ?>
    <?php if ($meta && $meta['description']) { ?>
    <div class="description">
      <?= $meta['description'] ?>
    </div>
    <?php } ?>
    <?php if ($meta && $meta['link']) { ?>
      <div class="description">
        <br><i class="linkify icon"></i>
        <a rel="nofollow noopener" href="<?= $meta['link'] ?>">
          <?= $meta['link'] ?>
        </a>
      </div>
    <?php } ?>
  </div>
  
</div>
