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

      


      
<style>
.block-animation {
  background-color:red;
  animation: loadin 1s forwards;
  background-color:rgba(105, 205, 100, 1);
}
@keyframes loadin {
    from {background-color:rgba(105, 205, 100, 1);}
    to {background-color:rgba(105, 205, 100, 0);}
}
</style>

<div class="ui body container">
  <h1 class="ui header">
    SteemDB
    <div class="sub header">
      Block explorer and database for the STEEM blockchain.
    </div>
  </h1>
  <div class="ui stackable grid">
    <div class="row">
      <div class="sixteen wide column">
        <!-- TradingView Widget BEGIN -->
        <script type="text/javascript" src="https://d33t3vvu2t2yu5.cloudfront.net/tv.js"></script>
        <script type="text/javascript">
        new TradingView.widget({
          "autosize": true,
          "symbol": "POLONIEX:STEEMBTC",
          "interval": "120",
          "timezone": "Etc/UTC",
          "theme": "White",
          "style": "1",
          "locale": "en",
          "toolbar_bg": "#f1f3f6",
          "enable_publishing": false,
          "hide_top_toolbar": true,
          "allow_symbol_change": true,
          "hideideas": true
        });
        </script>
        <!-- TradingView Widget END -->
      </div>
    </div>
    <div class="row">
      <div class="ten wide column">

        <div class="ui small dividing header">
          
          30-Day MVest Distribution
          <div class="sub header">
            Distribution of stake by the blockchain through various channels over 30 days.
          </div>
        </div>
        <div class="ui horizontal stacked segments">
          <div class="ui center aligned segment">
            <div class="ui <?php echo $this->largeNumber::color($totals['curation'])?> label" data-popup data-content="<?php echo number_format($totals['curation'], 3, ".", ",") ?> VESTS" data-variation="inverted" data-position="left center">
              <?php echo $this->largeNumber::format($totals['curation']); ?>
            </div>
            <div class="ui small header" style="margin-top: 0.5em;">
              <?php echo round($totals['curation'] / array_sum($totals) * 100, 1) ?>%<br>
              <a href="/labs/curation?grouping=monthly">
                <small>Curation</small>
              </a>
            </div>
          </div>
          <div class="ui center aligned segment">
            <div class="ui <?php echo $this->largeNumber::color($totals['author_rewards']['posts'])?> label" data-popup data-content="<?php echo number_format($totals['author_rewards']['posts'], 3, ".", ",") ?> VESTS" data-variation="inverted" data-position="left center">
              <?php echo $this->largeNumber::format($totals['author_rewards']['posts']); ?>
            </div>
            <div class="ui small header" style="margin-top: 0.5em;">
              <?php echo round($totals['author_rewards']['posts'] / array_sum($totals) * 100, 1) ?>%<br>
              <a href="/labs/author">
                <small>Authors</small>
              </a>
            </div>
          </div>
          <div class="ui center aligned segment">
            <div class="ui <?php echo $this->largeNumber::color($totals['author_rewards']['replies'])?> label" data-popup data-content="<?php echo number_format($totals['author_rewards']['replies'], 3, ".", ",") ?> VESTS" data-variation="inverted" data-position="left center">
              <?php echo $this->largeNumber::format($totals['author_rewards']['replies']); ?>
            </div>
            <div class="ui small header" style="margin-top: 0.5em;">
              <?php echo round($totals['author_rewards']['replies'] / array_sum($totals) * 100, 1) ?>%<br>
              <a href="/labs/author">
                <small>Commenters</small>
              </a>
            </div>
          </div>
          <div class="ui center aligned segment">
            <div class="ui <?php echo $this->largeNumber::color($totals['interest'])?> label" data-popup data-content="<?php echo number_format($totals['interest'], 3, ".", ",") ?> VESTS" data-variation="inverted" data-position="left center">
              <?php echo $this->largeNumber::format($totals['interest']); ?>
            </div>
            <div class="ui small header" style="margin-top: 0.5em;">
              <?php echo round($totals['interest'] / array_sum($totals) * 100, 1) ?>%<br>
              <a href="/accounts">
                <small>Interest</small>
              </a>
            </div>
          </div>
          <div class="ui center aligned segment">
            <div class="ui <?php echo $this->largeNumber::color($totals['witnesses'])?> label" data-popup data-content="<?php echo number_format($totals['witnesses'], 3, ".", ",") ?> VESTS" data-variation="inverted" data-position="left center">
              <?php echo $this->largeNumber::format($totals['witnesses']); ?>
            </div>
            <div class="ui small header" style="margin-top: 0.5em;">
              <?php echo round($totals['witnesses'] / array_sum($totals) * 100, 1) ?>%<br>
              <a href="/witnesses">
                <small>Witnesses</small>
              </a>
            </div>
          </div>
        </div>
        <div class="ui small dividing header">
          <a class="ui tiny blue basic button" href="/blocks" style="float:right">
            View more blocks
          </a>
          Recent Blockchain Activity
          <div class="sub header">
            Displaying most recent irreversible blocks.
          </div>
        </div>
        <div class="ui grid">
          <div class="two column row">
            <div class="column">
              <span class="ui horizontal blue basic label" data-props="head_block_number">
                <?= $props['head_block_number'] ?>
              </span>
              Current Height
            </div>
            <div class="column">
              <span class="ui horizontal orange basic label" data-props="reversible_blocks">
                <?= $props['head_block_number'] - $props['last_irreversible_block_num'] ?>
              </span>
              Reversable blocks awaiting consensus
            </div>
          </div>
        </div>
        <table class="ui small table" id="blockchain-activity">
          <thead>
            <tr>
              <th>Height</th>
              <th>Transactions</th>
              <th>Operations</th>
            </tr>
          </thead>
          <tbody>
            <tr class="loading center aligned">
              <td colspan="10">
                <div class="ui very padded basic segment">
                  <div class="ui active centered inline loader"></div>
                  <div class="ui header">
                    Waiting for new irreversible blocks
                  </div>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="six wide centered column">
        <div class="ui small dividing header">
          Metrics
          <div class="sub header">
            Witness Parameters, global properties and statistics
          </div>
        </div>
        <div class="ui horizontal stacked segments">
          <div class="ui center aligned segment">
            <div class="ui tiny statistic">
              <div class="value" data-props="steem_per_mvests">
                <?= $props['steem_per_mvests'] ?>
              </div>
              <div class="label">
                STEEM per MVest
              </div>
            </div>
          </div>
          <div class="ui center aligned segment">
            <div class="ui tiny statistic">
              <div class="value">
                <span data-state-feed="base">
                  <i class="notched circle loading icon"></i>
                </span>
              </div>
              <div class="label">
                per
                <span data-state-feed="quote">
                  <i class="notched circle loading icon"></i>
                </span>
              </div>
            </div>
          </div>
        </div>
        <div class="ui divider"></div>
        <div class="ui small header">
          Network Performance
        </div>
        <table class="ui small definition table" id="state">
          <tbody>
            <tr>
              <td class="eight wide">Transactions per second (24h)</td>
              <td>
                <?= $tx_per_sec ?> tx/sec
              </td>
            </tr>
            <tr>
              <td class="eight wide">Transactions per second (1h)</td>
              <td>
                <?= $tx1h_per_sec ?> tx/sec
              </td>
            </tr>
            <tr>
              <td>Transactions over 24h</td>
              <td>
                <?= $tx ?> txs
              </td>
            </tr>
            <tr>
              <td>Transactions over 1h</td>
              <td>
                <?= $tx1h ?> txs
              </td>
            </tr>
            <tr>
              <td>Operations over 24h</td>
              <td>
                <?= $op ?> ops
              </td>
            </tr>
            <tr>
              <td>Operations over 1h</td>
              <td>
                <?= $op1h ?> ops
              </td>
            </tr>
          </tbody>
        </table>
        <div class="ui small header">
          Consensus State
        </div>
        <table class="ui small definition table" id="state">
          <tbody>
            <tr>
              <td class="eight wide">Steem Inflation Rate</td>
              <td>
                <?= $inflation ?>
              </td>
            </tr>
            <tr>
              <td class="eight wide">Account Creation Fee</td>
              <td>
                <span data-state-witness-median="account_creation_fee">
                  <i class="notched circle loading icon"></i>
                </span>
              </td>
            </tr>
            <tr>
              <td>Maximum Block Size</td>
              <td>
                <span data-state-witness-median="maximum_block_size">
                  <i class="notched circle loading icon"></i>
                </span>
              </td>
            </tr>
            <tr>
              <td>SBD Interest Rate</td>
              <td>
                <span data-state-witness-median="sbd_interest_rate">
                  <i class="notched circle loading icon"></i>
                </span>
              </td>
            </tr>
          </tbody>
        </table>
        <div class="ui small header">
          Reward Pool
        </div>
        <table class="ui small definition table" id="global_props">
          <tbody>
            <?php foreach ($funds as $key => $value) { ?>
              <?php if (!$this->isIncluded($key, ['_id', 'id', 'name'])) { ?>
                <tr>
                  <td class="eight wide"><?= $key ?></td>
                  <td data-props="<?= $key ?>"><?= $value ?></td>
                </tr>
              <?php } ?>
            <?php } ?>
          </tbody>
        </table>

        <div class="ui small header">
          Global Properties
        </div>
        <table class="ui small definition table" id="global_props">
          <tbody>
            <?php foreach ($props as $key => $value) { ?>
              <?php if (!$this->isIncluded($key, ['id', 'steem_per_mvests', 'head_block_id', 'recent_slots_filled', 'head_block_number'])) { ?>
                <tr>
                  <td class="eight wide"><?= $key ?></td>
                  <td data-props="<?= $key ?>"><?= $value ?></td>
                </tr>
              <?php } ?>
            <?php } ?>
          </tbody>
        </table>
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

    
<script type="text/javascript">
   var sock = null;
   var ellog = null;

   window.onload = function() {

      var wsuri;
      ellog = document.getElementById('log');

      if (window.location.hostname === "localhost") {
         wsuri = "ws://localhost:8888";
      } else {
         wsuri = "ws://" + window.location.hostname + ":8888";
      }

      if ("WebSocket" in window) {
         sock = new WebSocket(wsuri);
      } else if ("MozWebSocket" in window) {
         sock = new MozWebSocket(wsuri);
      } else {
        //  log("Browser does not support WebSocket!");
      }

      if (sock) {
         sock.onopen = function() {
            // log("Connected to " + wsuri);
         }

         sock.onclose = function(e) {
            // log("Connection closed (wasClean = " + e.wasClean + ", code = " + e.code + ", reason = '" + e.reason + "')");
            sock = null;
         }

         sock.onmessage = function(e) {
            var data = JSON.parse(e.data);
            if(data.props) {
              $.each(data.props, function(key, value) {
                $("[data-props="+key+"]").html(value);
              });
            }
            if(data.state) {
              $.each(data.state.witness_schedule, function(key, value) {
                $("[data-state-witness="+key+"]").html(value);
              });
              $.each(data.state.witness_schedule.median_props, function(key, value) {
                $("[data-state-witness-median="+key+"]").html(value);
              });
              $.each(data.state.feed_price, function(key, value) {
                $("[data-state-feed="+key+"]").html(value);
              });
            }
            if(data.block) {
              var tbody = $("#blockchain-activity tbody"),
                  row = $("<tr class='block-animation'>"),
                  rows = tbody.find("tr"),
                  rowLimit = 19,
                  count = rows.length,
                  // Block Height
                  height_header = $("<div class='ui small header'>"),
                  height_header_link = $("<a>").attr("href", "/block/" + data.block.height).attr("target", "_blank").html("#"+data.block.height),
                  height_header_time = $("<div class='sub header'>").html(data.block.ts),
                  height = $("<td>").append(height_header.append(height_header_link, height_header_time)),
                  // Transactions
                  tx = $("<td>").append(data.block.opCount),
                  ops = $("<td>");
              $.each(data.block.opCounts, function(key, value) {
                var label = $("<span class='ui tiny basic label'>").append(key + " (" + value + ")");
                ops.append(label);
              });
              tbody.find("tr.loading").remove();
              row.append(height, tx, ops);
              tbody.prepend(row);
              if(count > rowLimit) {
                rows.slice(rowLimit-count).remove();
              }
            }
            // log(JSON.stringify(data));
         }
      }
   };

  //  function broadcast() {
  //     var account = document.getElementById('account').value;
  //     if (sock) {
  //        sock.send(account);
  //        log("Subscribed account: " + account);
  //     } else {
  //        log("Not connected.");
  //     }
  //  };

   function log(m) {
      ellog.innerHTML += m + '\n';
      ellog.scrollTop = ellog.scrollHeight;
   };
</script>

  </body>
</html>
