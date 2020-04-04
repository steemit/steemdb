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
  <div class="ui top aligned stackable grid container">
    <div class="row">
      <div class="sixteen wide column">
        <div class="ui huge header">
          Witnesses
          <div class="sub header">
            DPOS elected witnesses and relevant historical information.
          </div>
        </div>
        <div style="overflow-x:auto;">
          <div class="ui top attached tabular menu">
            <a class="active item" href="/witnesses">Witnesses</a>
            <a class="item" href="/witness/history">History</a>
            <a class="item" href="/witness/misses">Misses</a>
          </div>
          <div class="ui bottom attached segment">
            <div class="ui active tab">
              <table class="ui small unstackable table">
                <thead>
                  <tr>
                    <th class="right aligned">Rank</th>
                    <th>Witness</th>
                    <th>Votes</th>
                    <th class="center aligned">
                      Weekly<br>
                      &amp; Total<br>
                      Misses
                    </th>
                    <th>Price Feed</th>
                    <th>
                      Reg Fee<br>
                      APR<br>
                      Block Size
                    </th>
                    <th>Version</th>
                    <th>VESTS</th>
                  </tr>
                </thead>
                <tbody>
                  <?php $v159407990739148203481iterator = $witnesses; $v159407990739148203481incr = 0; $v159407990739148203481loop = new stdClass(); $v159407990739148203481loop->self = &$v159407990739148203481loop; $v159407990739148203481loop->length = count($v159407990739148203481iterator); $v159407990739148203481loop->index = 1; $v159407990739148203481loop->index0 = 1; $v159407990739148203481loop->revindex = $v159407990739148203481loop->length; $v159407990739148203481loop->revindex0 = $v159407990739148203481loop->length - 1; ?><?php foreach ($v159407990739148203481iterator as $witness) { ?><?php $v159407990739148203481loop->first = ($v159407990739148203481incr == 0); $v159407990739148203481loop->index = $v159407990739148203481incr + 1; $v159407990739148203481loop->index0 = $v159407990739148203481incr; $v159407990739148203481loop->revindex = $v159407990739148203481loop->length - $v159407990739148203481incr; $v159407990739148203481loop->revindex0 = $v159407990739148203481loop->length - ($v159407990739148203481incr + 1); $v159407990739148203481loop->last = ($v159407990739148203481incr == ($v159407990739148203481loop->length - 1)); ?>
                    <tr class="<?= $witness->row_status ?>">
                      <td class="right aligned collapsing">
                        <?php if ($v159407990739148203481loop->index <= 19) { ?>
                          <strong><?= $v159407990739148203481loop->index ?></strong>
                        <?php } else { ?>
                          <?= $v159407990739148203481loop->index ?>
                        <?php } ?>
                      </td>
                      <td>
                        <div class="ui header">
                          <a href="/@<?= $witness->owner ?>">
                            <?= $witness->owner ?>
                          </a>
                          <div class="sub header">
                            <a href="<?= $witness->url ?>">
                              witness url
                            </a>
                          </div>
                        </div>
                      </td>
                      <td class="collapsing">
                        <div class="ui header">
                          <?php echo $this->largeNumber::format($witness->votes); ?>
                        </div>
                      </td>
                      <td class="center aligned">
                        <a href="/@<?= $witness->owner ?>/missed" class="ui small header">
                          <?php if ($witness->invalid_signing_key) { ?>
                          <i class="warning sign icon" data-popup data-title="Witness Disabled" data-content="This witness does not have a signing key either at the owners request or because too many blocks have been missed."></i>
                          <?php } ?>
                          <div class="content">
                            <?php if ($witness->misses_7day > 0) { ?>
                              <div class="ui tiny grey label">
                                <?= '+' . $witness->misses_7day ?>
                              </div>
                            <?php } else { ?>
                              ~
                            <?php } ?>
                            <div class="sub header">
                              <small><?= $witness->total_missed ?></small>
                            </div>
                          </div>
                        </a>
                      </td>
                      <td>
                        <div class="ui header">
                          <?php if ($witness->sbd_exchange_rate->base === '0.000 STEEM' || $witness->last_sbd_exchange_update_late) { ?><i class="warning sign icon" data-popup data-title="Outdated Price Feed" data-content="This witness has not submitted a price feed update in over a week."></i><?php } ?>
                          <div class="content">
                            <?= $witness->sbd_exchange_rate->base ?>
                            <?php if ($witness->sbd_exchange_rate->quote != '1.000 STEEM') { ?>
                            (<?php echo round((1 - 1/explode(" ", $witness->sbd_exchange_rate['quote'])[0]) * 100, 1) ?>%)
                            <?php } ?>
                            <div class="sub header">
                              <?= $witness->sbd_exchange_rate->quote ?><br>
                              <?php if ('' . $witness->last_sbd_exchange_update > 0) { ?>
                                <?php echo $this->timeAgo::mongo($witness->last_sbd_exchange_update); ?>
                              <?php } else { ?>
                                Never
                              <?php } ?>
                            </div>
                          </div>
                        </div>
                      </td>
                      <td>
                        <?= $witness->props->account_creation_fee ?>
                        <br>
                        <?= $witness->props->sbd_interest_rate / 100 ?><small>%</small> APR
                        <br>
                        <?= $witness->props->maximum_block_size ?>
                      </td>
                      <td>
                        <?= $witness->running_version ?>
                      </td>
                      <td>
                        <?= $this->partial('_elements/vesting_shares', ['current' => $witness->account[0]]) ?>
                      </td>
                    </tr>
                  <?php $v159407990739148203481incr++; } ?>
                </tbody>
              </table>
            </div>
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

    
  </body>
</html>
