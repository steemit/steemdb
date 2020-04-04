<h3 class="ui dividing header">Tags</h3>
<div class="ui relaxed list">
  <?php foreach ($comment->metadata('tags') as $tag) { ?>
  <div class="item">
    <?= $tag ?>
  </div>
  <?php } ?>
</div>
