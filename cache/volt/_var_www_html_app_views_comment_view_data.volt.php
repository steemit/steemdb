<a class="ui primary button" href="/<?= $comment->category ?>/@<?= $comment->author ?>/<?= $comment->permlink ?>/json">View JSON</a>
<?php $this->partial('_elements/definition_table', ['data' => $comment->toArray()]); ?>
