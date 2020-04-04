<!DOCTYPE html>
<html>
  <head>
  <meta name='viewport' content='initial-scale=1,maximum-scale=1,user-scalable=no,minimal-ui' />
  <title>SteemDB - STEEM Blockchain Explorer</title>
  <?php if (isset($post)) { ?>
  <link rel="canonical" href="https://steemit.com/<?= $post->category ?>/@<?= $post->author ?>/<?= $post->permlink ?>"/>
  <?php } ?>
  <?php if (isset($posts) && isset($posts[0])) { ?>
  <link rel="canonical" href="https://steemit.com/<?= $posts[0]->category ?>/@<?= $posts[0]->author ?>/<?= $posts[0]->permlink ?>"/>
  <?php } ?>
  <style>
    .ui.vertical.sidebar.menu {
      padding-top: 3em !important;
    }
    body.pushable>.pusher {
      background: #efefef;
    }
    .ui.vertical.stripe {
      padding: 3em 0em;
    }
    .ui.vertical.stripe h3 {
      font-size: 2em;
    }
    .ui.vertical.stripe .button + h3,
    .ui.vertical.stripe p + h3 {
      margin-top: 3em;
    }
    .ui.vertical.stripe .floated.image {
      clear: both;
    }
    .ui.vertical.stripe p {
      font-size: 1.33em;
    }
    .ui.vertical.stripe .horizontal.divider {
      margin: 3em 0em;
    }
    .quote.stripe.segment {
      padding: 0em;
    }
    .quote.stripe.segment .grid .column {
      padding-top: 5em;
      padding-bottom: 5em;
    }
    .footer.segment {
      padding: 5em 0em;
    }
    .footer.segment a {
      color: #fff;
      text-decoration: underline;
    }
    .comment img,
    .markdown img {
      max-width: 100%;
      height:auto;
      display: block;
    }
    .markdown {
      font-size: 1.25em;
    }
    .markdown div.pull-left {
      float: left;
      padding-right: 1rem;
      max-width: 50%;
    }
    .markdown div.pull-right {
      float: right;
      padding-left: 1rem;
      max-width: 50%;
    }
    .markdown blockquote, .markdown blockquote p {
      line-height: 1.6;
      color: #8a8a8a;
    }
    .markdown blockquote {
      margin: 0 0 1rem;
      padding: .53571rem 1.19048rem 0 1.13095rem;
      border-left: 1px solid #cacaca;
    }
    .markdown code {
      white-space: pre;
      font-family: Consolas,Liberation Mono,Courier,monospace;
      display: block;
      padding: 10px;
      background: #f4f4f4;
      border-radius: 3px;
    }
    .ui.comments {
      max-width: auto;
    }
    .ui.comments .comment .comments {
      padding-left: 3em;
    }
    .definition.table td.wide {
      overflow-x: auto;
    }
    .ui.body.container {
      margin: 3em 0;
    }
    @media only screen and (min-width: 768px) {
      body .ui.table:not(.unstackable) tr>td.mobile.visible,
      body .ui.table:not(.unstackable) tr>th.mobile.visible,
      .mobile.visible {
        display: none
      }
    }
    @media only screen and (max-width: 767px) {
      .ui.tabular.menu {
        overflow-y: scroll;
      }
      body .ui.table:not(.unstackable) tr>td.mobile.hidden,
      body .ui.table:not(.unstackable) tr>th.mobile.hidden,
      .mobile.hidden {
        display: none !important;
      }
    }
  </style>
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/semantic-ui/2.2.2/semantic.min.css">
  <link rel="stylesheet" href="/bower/plottable/plottable.css">
</head>

  <body>

    <div class="ui fixed inverted blue main menu">
  <div class="ui container">
    <a class="launch icon item">
      <i class="content icon"></i>
    </a>

    <div class="right menu">
      <div class="ui category search item">
        <div class="ui icon input">
          <input class="prompt" type="text" placeholder="Search accounts...">
          <i class="search icon"></i>
        </div>
        <div class="results"></div>
      </div>
    </div>
  </div>
</div>
<!-- Following Menu -->
<div class="ui blue inverted top fixed mobile hidden menu">
  <div class="ui container">
    <div class="item" style="background: white">
      <div class="ui floating labeled dropdown">
        <img class="ui avatar image" style="border-radius: 0; width: 24px; height: 24px" src="https://steemdb.com/explorers/steem.png"/>
        <i class="dropdown black icon"></i>
        <div class="menu">
          <a class="active item" href="https://steemdb.com<?= strip_tags($this->router->getRewriteUri()) ?>">
            <img class="ui avatar image" style="border-radius: 0; width: 24px; height: 24px" src="https://steemdb.com/explorers/steem.png"/>
            steem
          </a>
          <a class="item" href="https://golosdb.com<?= strip_tags($this->router->getRewriteUri()) ?>">
            <img class="ui avatar image" style="border-radius: 0; width: 24px; height: 24px" src="https://steemdb.com/explorers/golos.jpg"/>
            golos
          </a>
          <a class="item" href="https://peerplaysdb.com<?= strip_tags($this->router->getRewriteUri()) ?>">
            <img class="ui avatar image" style="border-radius: 0; width: 24px; height: 24px" src="https://steemdb.com/explorers/ppy.png"/>
            peerplays
          </a>
          <a class="item" href="https://decent-db.com<?= strip_tags($this->router->getRewriteUri()) ?>">
            <img class="ui avatar image" style="border-radius: 0; width: 24px; height: 24px" src="https://steemdb.com/explorers/dct.png"/>
            decent
          </a>
          <a class="item" href="https://muse-db.com<?= strip_tags($this->router->getRewriteUri()) ?>">
            <img class="ui avatar image" style="border-radius: 0; width: 24px; height: 24px" src="https://steemdb.com/explorers/muse.png"/>
            muse
          </a>
        </div>
      </div>
    </div>
    <a href="/" class="header <?= (($this->router->getControllerName() == 'index') ? 'active' : '') ?> item">SteemDB</span>
    <a href="/accounts" class="<?= (($this->router->getControllerName() == 'account' || $this->router->getControllerName() == 'accounts') ? 'active' : '') ?> item">accounts</a>
    <a href="/apps" class="<?= (($this->router->getControllerName() == 'apps') ? 'active' : '') ?> item">apps</a>
    <a href="/comments/daily" class="<?= (($this->router->getControllerName() == 'comments') ? 'active' : '') ?> item">posts</a>
    <a href="/witnesses" class="<?= (($this->router->getControllerName() == 'witness') ? 'active' : '') ?> item">witnesses</a>
    <!-- <a href="https://blog.steemdb.com" class="item">updates</a> -->
    <a href="/labs" class="<?= (($this->router->getControllerName() == 'labs') ? 'active' : '') ?> item">labs</a>
    <div class="right menu">
      <div class="item">
        <a href="https://steemit.com/?r=jesta">
          <small>Create Account</small>
        </a>
      </div>
      <div class="ui category search item">
        <div class="ui icon input">
          <input class="prompt" type="text" placeholder="Search accounts...">
          <i class="search icon"></i>
        </div>
        <div class="results"></div>
      </div>
    </div>
  </div>
</div>

<!-- Sidebar Menu -->
<div class="ui vertical inverted sidebar menu">
  <a href="/" class="<?= (($this->router->getControllerName() == 'comment') ? 'active' : '') ?> item">posts</a>
  <a href="/accounts" class="<?= (($this->router->getControllerName() == 'account' || $this->router->getControllerName() == 'accounts') ? 'active' : '') ?> item">accounts</a>
  <a href="/witnesses" class="<?= (($this->router->getControllerName() == 'witness') ? 'active' : '') ?> item">witnesses</a>
 <!-- <a href="https://blog.steemdb.com" class="item">updates</a> -->
  <a href="/labs" class="<?= (($this->router->getControllerName() == 'labs') ? 'active' : '') ?> item">labs</a>
</div>


    <!-- Page Contents -->
    <div class="pusher" style="padding-top: 3em">

      


      



      <?php if ($this->flashSession->has()) { ?>
      <div class="ui container">
        <div class="ui error message">
          <?php $this->flashSession->output() ?>
        </div>
      </div>
      <?php } ?>

      
  <div class="ui container">
    <div class="ui grid">
      <div class="one column row">
        <div class="column">
          <div class="ui large header" style="margin-top: 50px">
            Pending Payout Review
            <div class="sub header">
              Posts with 12 hours or less of voting remaining (downvote only timeframe), sorted by payout amount.
            </div>
          </div>
          <div class="ui internally celled stackable grid">
            <?php $v156554960253913002181iterated = false; ?><?php $v156554960253913002181iterator = $comments; $v156554960253913002181incr = 0; $v156554960253913002181loop = new stdClass(); $v156554960253913002181loop->self = &$v156554960253913002181loop; $v156554960253913002181loop->length = count($v156554960253913002181iterator); $v156554960253913002181loop->index = 1; $v156554960253913002181loop->index0 = 1; $v156554960253913002181loop->revindex = $v156554960253913002181loop->length; $v156554960253913002181loop->revindex0 = $v156554960253913002181loop->length - 1; ?><?php foreach ($v156554960253913002181iterator as $comment) { ?><?php $v156554960253913002181loop->first = ($v156554960253913002181incr == 0); $v156554960253913002181loop->index = $v156554960253913002181incr + 1; $v156554960253913002181loop->index0 = $v156554960253913002181incr; $v156554960253913002181loop->revindex = $v156554960253913002181loop->length - $v156554960253913002181incr; $v156554960253913002181loop->revindex0 = $v156554960253913002181loop->length - ($v156554960253913002181incr + 1); $v156554960253913002181loop->last = ($v156554960253913002181incr == ($v156554960253913002181loop->length - 1)); ?><?php $v156554960253913002181iterated = true; ?>
              <div class="row">
                <div class="one wide mobile hidden column">
                  <div class="ui small center aligned header">
                    <svg height="16" enable-background="new 0 0 33 33" version="1.1" viewBox="0 0 33 33" xml:space="preserve" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><g id="Chevron_Up_Circle"><circle cx="16" cy="16" r="15"  style="fill: #4ba2f2; stroke: #4ba2f2;"></circle><path d="M16.699,11.293c-0.384-0.38-1.044-0.381-1.429,0l-6.999,6.899c-0.394,0.391-0.394,1.024,0,1.414 c0.395,0.391,1.034,0.391,1.429,0l6.285-6.195l6.285,6.196c0.394,0.391,1.034,0.391,1.429,0c0.394-0.391,0.394-1.024,0-1.414 L16.699,11.293z" fill="#fff"></path></g></svg>
                    <?= $comment->net_votes ?>
                    <div class="sub header">#<?= $v156554960253913002181loop->index ?></div>
                  </div>
                </div>
                <div class="twelve wide column">
                  <div class="ui large header">
                    <a href="<?= $comment->url ?>" style="color: #555">
                      <?= $comment->title ?>
                    </a>
                    <div class="sub header">
                      <span class="ui circular label" style="margin-left: 0">
                        <?php echo $this->reputation::number($comment->author_reputation) ?>
                      </span>
                      <a href="/@<?= $comment->author ?>">
                        <?= $comment->author ?>
                      </a>
                      in
                      <?php if (isset($sort) && isset($date)) { ?>
                      <a href="/posts/<?= $comment->category ?>/<?= ($sort ? $sort : 'earnings') ?>/<?= date('Y-m-d', $date) ?>">
                        #<?= $comment->category ?>
                      </a>
                      <?php } else { ?>
                      <a href="/posts/<?= $comment->category ?>/earnings/<?= date('Y-m-d') ?>">
                        #<?= $comment->category ?>
                      </a>
                      <?php } ?>
                      <span class="mobile hidden">&mdash;</span>
                      <br class="mobile visible">
                      <span class="ui small left floated green header mobile visible">
                        $<?= $comment->total_payout_value ?>
                        <span class="sub header">
                          (+<?php echo $this->largeNumber::format($comment->pending_payout_value); ?> Pending)
                        </span>
                      </span>
                      payout in <?php echo $this->timeAgo::mongo($comment->cashout_time); ?>
                      &mdash;
                      <a target="_blank" href="https://steemit.com<?= $comment->url ?>">steemit.com</a>
                      &mdash;
                      <a target="_blank" href="https://beta.chainbb.com<?= $comment->url ?>">chainbb.com</a>
                    </div>
                  </div>
                </div>
                <div class="three wide center mobile hidden aligned column">
                  <div class="ui green header">
                    $<?= $comment->total_payout_value ?>
                    <div class="sub header">
                      +<?php echo $this->largeNumber::format($comment->pending_payout_value); ?> Pending
                    </div>
                  </div>
                </div>
              </div>
            <?php $v156554960253913002181incr++; } if (!$v156554960253913002181iterated) { ?>
              <div class="row">
                <div class="center aligned column">
                  <div class="ui message">
                    <div class="header">No posts found</div>
                  </div>
                </div>
              </div>
            <?php } ?>
          </div>
        </div>
      </div>
    </div>
  </div>


      <div class="ui container">
  <div class="ui basic very padded segment">
    <center>
      <small>
        * All Steem Power & VEST calculations are done using the current conversion rate, not a historical rate. This may cause some calculations to be incorrect.
      </small>
    </center>
  </div>
</div>
<div class="ui inverted vertical footer segment">
  <div class="ui container">
    <div class="ui stackable inverted divided equal height stackable grid">
      <div class="sixteen wide center aligned column">
        <h4 class="ui inverted header">
          created by @ray.wu
        </h4>
        <!-- <p>If you'd like to support this project, <a href="https://steemit.com/~witnesses">vote <strong>jesta</strong> as witness.</a></p> -->
      </div>
    </div>
  </div>
</div>


    </div>

    <script type="text/javascript" src="https://code.jquery.com/jquery-3.1.0.min.js"></script>
<script type="text/javascript" src="https://cdnjs.cloudflare.com/ajax/libs/semantic-ui/2.2.2/semantic.min.js"></script>
<script type="text/javascript" src="/bower/d3/d3.min.js"></script>
<script type="text/javascript" src="/bower/plottable/plottable.min.js"></script>
<script type="text/javascript" src="/js/semantic-tablesort.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/Chart.js/2.4.0/Chart.min.js"></script>


<script>
$(document)
  .ready(function() {

    $('.ui.category.search')
      .search({
        apiSettings: {
          url: '/search?q={query}'
        },
        debug: true,
        type: 'category'
      })
    ;

    $('.ui.sortable.table').tablesort();

    // create sidebar and attach to menu open
    $('.ui.sidebar')
      .sidebar('attach events', '.launch.item')
    ;

    $('.ui.dropdown')
      .dropdown({

      })
    ;

    $('[data-popup]')
      .popup({
        hoverable: true
      })
    ;

    $('.ui.dropdown.tags')
      .dropdown({
        onChange: function(value, text, $choice) {
          var selectedSort = $("#selectedSort").val(),
              selectedDate = $("#selectedDate").val();
          window.location.href = value + '/' + selectedSort + '/' + selectedDate;
        },
        apiSettings: {
          url: '/api/tags/{query}'
        }
      });

  })
;
</script>
<script>
  (function(i,s,o,g,r,a,m){i['GoogleAnalyticsObject']=r;i[r]=i[r]||function(){
  (i[r].q=i[r].q||[]).push(arguments)},i[r].l=1*new Date();a=s.createElement(o),
  m=s.getElementsByTagName(o)[0];a.async=1;a.src=g;m.parentNode.insertBefore(a,m)
  })(window,document,'script','https://www.google-analytics.com/analytics.js','ga');

  ga('create', 'UA-81121004-2', 'auto');
  ga('send', 'pageview');

</script>

    
  </body>
</html>
