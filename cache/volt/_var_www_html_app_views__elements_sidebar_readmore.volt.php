<div class="ui divided relaxed  list">
  <div class="item">
    <strong>
      More posts by
      <a href="/@<?= $comment->author ?>">
        <?= $comment->author ?>
      </a>
    </strong>
  </div>
  <?php foreach ($posts as $post) { ?>
    <?php if ($post->url === $comment->url) { ?>
      <?php continue; ?>
    <?php } ?>
    <div class="item">
      <?php echo $this->timeAgo::mongo($post->created); ?><br>
      <a href="<?= $post->url ?>">
        <?= $post->title ?>
      </a>

    </div>
  <?php } ?>
</div>
