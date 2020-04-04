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

      
<div class="ui vertical stripe segment">
  <div class="ui stackable grid container">
    <div class="row">
      <div class="twelve wide column" id="main-context">
        <div class="ui top attached menu">
          <?= $this->tag->linkTo([['for' => 'account-view', 'account' => $account->name], '<i class=\'home icon\'></i>', 'class' => 'icon item' . (($this->router->getActionName() == 'view' ? ' active' : ''))]) ?>
          <div class="ui dropdown item">
            Activity
            <i class="dropdown icon"></i>
            <div class="menu">
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'posts'], 'Posts', 'class' => 'item' . (($this->router->getActionName() == 'posts' ? ' active' : ''))]) ?>
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'votes'], 'Votes', 'class' => 'item' . (($this->router->getActionName() == 'votes' ? ' active' : ''))]) ?>
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'replies'], 'Replies', 'class' => 'item' . (($this->router->getActionName() == 'replies' ? ' active' : ''))]) ?>
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'reblogs'], 'Reblogs', 'class' => 'item' . (($this->router->getActionName() == 'reblogs' ? ' active' : ''))]) ?>
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'authoring'], 'Rewards', 'class' => 'item' . (($this->router->getActionName() == 'authoring' ? ' active' : ''))]) ?>
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'transfers'], 'Transfers', 'class' => 'item' . (($this->router->getActionName() == 'transfers' ? ' active' : ''))]) ?>
            </div>
          </div>
          <div class="ui dropdown item">
            Social
            <i class="dropdown icon"></i>
            <div class="menu">
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'followers'], 'Followers', 'class' => 'item' . (($this->router->getActionName() == 'followers' ? ' active' : ''))]) ?>
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'following'], 'Following', 'class' => 'item' . (($this->router->getActionName() == 'following' ? ' active' : ''))]) ?>
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'reblogged'], 'Reblogged', 'class' => 'item' . (($this->router->getActionName() == 'reblogged' ? ' active' : ''))]) ?>
            </div>
          </div>
          <div class="ui dropdown item">
            Witness
            <i class="dropdown icon"></i>
            <div class="menu">
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'witness'], 'Voting', 'class' => 'item' . (($this->router->getActionName() == 'witness' ? ' active' : ''))]) ?>
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'blocks'], 'Blocks', 'class' => 'item' . (($this->router->getActionName() == 'blocks' ? ' active' : ''))]) ?>
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'missed'], 'Missed', 'class' => 'item' . (($this->router->getActionName() == 'missed' ? ' active' : ''))]) ?>
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'props'], 'Props', 'class' => 'item' . (($this->router->getActionName() == 'props' ? ' active' : ''))]) ?>
              <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'proxied'], 'Proxied', 'class' => 'item' . (($this->router->getActionName() == 'proxied' ? ' active' : ''))]) ?>
            </div>
          </div>
          <?= $this->tag->linkTo([['for' => 'account-view-section', 'account' => $account->name, 'action' => 'data'], 'Data', 'class' => 'item' . (($this->router->getActionName() == 'data' ? ' active' : ''))]) ?>
        </div>
        <?php if ($chart) { ?>
        <div class="ui attached segment">
          <svg width="100%" height="200px" id="account-<?= $this->router->getActionName() ?>"></svg>
        </div>
        <?php } ?>
        <div class="ui bottom attached secondary segment">
          <?php $this->partial('account/view/' . $this->router->getActionName()); ?>
        </div>
      </div>
      <div class="four wide column">
        <div class="ui sticky">
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

          <div class="ui small indicating progress">
            <div class="label">
              <?= $live[0]['voting_power'] / 10000 * 100 ?>% Voting Power
            </div>
            <div class="bar">
              <div class="progress"></div>
            </div>
          </div>
          <div class="ui list">
            <div class="item">
              <a href="https://steemit.com/@<?= $account->name ?>" class="ui fluid primary icon small basic button" target="_blank">
                <i class="external icon"></i>
                View Account on steemit.com
              </a>
            </div>
            <div class="item">
              <a href="https://steemd.com/@<?= $account->name ?>" class="ui fluid teal icon small basic button" target="_blank">
                <i class="external icon"></i>
                View Account on steemd.com
              </a>
            </div>
          </div>
          <table class="ui small definition table">
            <tbody>
              <tr>
                <td>VESTS</td>
                <td>
                  <?= $this->partial('_elements/vesting_shares', ['current' => $account]) ?>
                </td>
              </tr>
              <tr <?php if ($account->vesting_withdraw_rate && $account->vesting_withdraw_rate > 1) { ?>data-popup data-html="<table class='ui small definition table'><tr><td>Power Down - Rate</td><td>-<?php echo $this->convert::vest2sp($current->vesting_withdraw_rate, " SP"); ?></td></tr><tr><td>Power Down - Datetime</td><td><?php echo gmdate("Y-m-d H:i:s e", (string) $account->next_vesting_withdrawal / 1000) ?></td></tr></table>" data-position="left center" data-variation="very wide"<?php } ?>>
                <td>SP</td>
                <td>
                  <div class="ui tiny header">
                    <?php echo $this->convert::vest2sp($current->vesting_shares); ?>
                  </div>
                </td>
              </tr>
              <tr data-popup data-html="<table class='ui small definition table'><tr><td>Balance</td><td><?php echo number_format($account->balance, 3, '.', ','); ?></td></tr><tr><td>Savings Balance</td><td><?php echo number_format($account->savings_balance, 3, '.', ','); ?></td></tr><?php if ($account->vesting_withdraw_rate && $account->vesting_withdraw_rate > 1 && !$account->withdraw_routes) { ?><tr><td>Power Down - Rate</td><td>+<?php echo $this->convert::vest2sp($current->vesting_withdraw_rate, " STEEM"); ?></td></tr><tr><td>Power Down - Datetime</td><td><?php echo gmdate("Y-m-d H:i:s e", (string) $account->next_vesting_withdrawal / 1000) ?></td></tr><?php } ?></table>" data-position="left center" data-variation="very wide">
                <td>STEEM</td>
                <td>
                  <div class="ui tiny header">
                    <?php echo number_format($account->total_balance, 3, '.', ','); ?>
                  </div>
                </td>
              </tr>
              <tr data-popup data-html="<table class='ui small definition table'><tr><td>Balance</td><td><?php echo number_format($account->sbd_balance, 3, '.', ','); ?></td></tr><tr><td>Savings Balance</td><td><?php echo number_format($account->savings_sbd_balance, 3, '.', ','); ?></td></tr><tr><td>Next Interest (10% APY)</td><td><?php echo gmdate("Y-m-d H:i:s e", strtotime("+30 days", (string) $account->sbd_last_interest_payment / 1000)); ?></td></tr></table>" data-position="left center" data-variation="very wide">
                <td>SBD</td>
                <td>
                  <div class="ui tiny header">
                    <?php echo number_format($account->total_sbd_balance, 3, '.', ','); ?>
                    <div class="sub header">
                    </div>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
          <div class="ui tiny centered header">
            <span class="sub header">
              Account snapshot taken
              <?php echo $this->timeAgo::mongo($account->scanned); ?>
            </span>
          </div>
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

    
  <?php if (isset($chart)) { ?>
    <?php $this->partial('charts/account/' . $this->router->getActionName()); ?>
  <?php } ?>
  <script>
    $('.ui.indicating.progress')
      .progress({
        percent: <?= $live[0]['voting_power'] / 100 ?>
      });
    $('.tabular.menu .item')
      .tab({

      });
    $('.ui.sticky')
      .sticky({
        context: '#main-context',
        offset: 90
      });
    $(".ui.sortable.table").tablesort();
  </script>

  </body>
</html>
