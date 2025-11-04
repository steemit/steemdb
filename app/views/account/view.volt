{% extends 'layouts/default.volt' %}

{% block content %}
<div class="ui vertical stripe segment">
  <div class="ui stackable mobile reversed grid container">
    <div class="twelve wide column" id="main-context">
      <div class="ui top attached menu">
        {{ link_to(["for": "account-view", "account": account.name], "<i class='home icon'></i>", "class": "icon item" ~ (router.getActionName() == "view" ? " active" : "")) }}
        <div class="ui dropdown item">
          Activity
          <i class="dropdown icon"></i>
          <div class="menu">
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "posts"], "Posts", "class": "item" ~ (router.getActionName() == "posts" ? " active" : "")) }}
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "votes"], "Votes", "class": "item" ~ (router.getActionName() == "votes" ? " active" : "")) }}
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "replies"], "Replies", "class": "item" ~ (router.getActionName() == "replies" ? " active" : "")) }}
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "reblogs"], "Reblogs", "class": "item" ~ (router.getActionName() == "reblogs" ? " active" : "")) }}
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "authoring"], "Rewards", "class": "item" ~ (router.getActionName() == "authoring" ? " active" : "")) }}
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "transfers"], "Transfers", "class": "item" ~ (router.getActionName() == "transfers" ? " active" : "")) }}
          </div>
        </div>
        <div class="ui dropdown item">
          Social
          <i class="dropdown icon"></i>
          <div class="menu">
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "followers"], "Followers", "class": "item" ~ (router.getActionName() == "followers" ? " active" : "")) }}
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "following"], "Following", "class": "item" ~ (router.getActionName() == "following" ? " active" : "")) }}
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "reblogged"], "Reblogged", "class": "item" ~ (router.getActionName() == "reblogged" ? " active" : "")) }}
          </div>
        </div>
        <div class="ui dropdown item">
          Witness
          <i class="dropdown icon"></i>
          <div class="menu">
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "witness"], "Voting", "class": "item" ~ (router.getActionName() == "witness" ? " active" : "")) }}
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "blocks"], "Blocks", "class": "item" ~ (router.getActionName() == "blocks" ? " active" : "")) }}
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "missed"], "Missed", "class": "item" ~ (router.getActionName() == "missed" ? " active" : "")) }}
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "props"], "Props", "class": "item" ~ (router.getActionName() == "props" ? " active" : "")) }}
            {{ link_to(["for": "account-view-section", "account": account.name, "action": "proxied"], "Proxied", "class": "item" ~ (router.getActionName() == "proxied" ? " active" : "")) }}
          </div>
        </div>
        {{ link_to(["for": "account-view-section", "account": account.name, "action": "data"], "Data", "class": "item" ~ (router.getActionName() == "data" ? " active" : "")) }}
      </div>
      {% if chart %}
      <div class="ui attached segment">
        <svg width="100%" height="200px" id="account-{{ router.getActionName() }}"></svg>
      </div>
      {% endif %}
      <div class="ui bottom attached secondary segment" style="overflow-x: scroll;">
        {% include "account/view/" ~ router.getActionName() %}
      </div>
    </div>
    <div class="four wide column">
      <div class="ui">
        {% include '_elements/cards/account.volt' %}
        <div class="ui small indicating progress" id="vp_progress">
          <div class="label">
            {{ live[0]['voting_power'] / 10000 * 100 }}% Voting Power
          </div>
          <div class="bar">
            <div class="progress"></div>
          </div>
        </div>
        <div class="ui small indicating progress" id="mana_progress">
          <div class="label" data-popup data-html="<table class='ui small definition table'><tr><td>Current Mana</td><td>{{ rc['rc_manabar']['current_mana'] }}</td></tr><tr><td>Total Mana</td><td>{{ rc['max_rc'] }}</td></tr></table>" data-position="bottom center">
            <?php echo round($rc['rc_manabar']['current_mana'] / $rc['max_rc'] * 100, 2);?>% Mana
          </div>
          <div class="bar">
            <div class="progress"></div>
          </div>
        </div>
        <div class="ui list">
          <div class="item">
            <a href="https://steemit.com/@{{ account.name }}" class="ui fluid primary icon small basic button" target="_blank">
              <i class="external icon"></i>
              View Account on steemit.com
            </a>
          </div>
        </div>
        <table class="ui small definition table">
          <tbody>
            <tr>
              <td>VESTS</td>
              <td>
                {{ partial("_elements/vesting_shares", ['current': account]) }}
              </td>
            </tr>
            <tr {% if account.vesting_withdraw_rate and account.vesting_withdraw_rate > 1 %}data-popup data-html="<table class='ui small definition table'><tr><td>Power Down - Rate</td><td>-<?php echo $this->convert::vest2sp($current->vesting_withdraw_rate, " SP"); ?></td></tr><tr><td>Power Down - Datetime</td><td><?php echo gmdate("Y-m-d H:i:s e", (string) $account->next_vesting_withdrawal / 1000) ?></td></tr></table>" data-position="left center" data-variation="very wide"{% endif %}>
              <td>Total SP</td>
              <td>
                <div class="ui tiny header">
                  <?php echo $this->convert::vest2sp($current->vesting_shares); ?>
                </div>
              </td>
            </tr>
            <tr>
              <td>Delegated SP</td>
              <td>
                <div class="ui tiny header">
                  <?php echo $this->convert::vest2sp($current->delegated_vesting_shares); ?>
                </div>
              </td>
            </tr>
            <tr>
              <td>Received SP</td>
              <td>
                <div class="ui tiny header">
                  <?php echo $this->convert::vest2sp($current->received_vesting_shares); ?>
                </div>
              </td>
            </tr>
            <tr>
              <td>Effective SP</td>
              <td>
                <div class="ui tiny header">
                  <?php echo $this->convert::vest2sp($current->vesting_shares + $current->received_vesting_shares - $current->delegated_vesting_shares); ?>
                </div>
              </td>
            </tr>
            <tr data-popup data-html="<table class='ui small definition table'><tr><td>Balance</td><td><?php echo number_format($account->balance, 3, '.', ','); ?></td></tr><tr><td>Savings Balance</td><td><?php echo number_format($account->savings_balance, 3, '.', ','); ?></td></tr>{% if account.vesting_withdraw_rate and account.vesting_withdraw_rate > 1 and not account.withdraw_routes %}<tr><td>Power Down - Rate</td><td>+<?php echo $this->convert::vest2sp($current->vesting_withdraw_rate, " STEEM"); ?></td></tr><tr><td>Power Down - Datetime</td><td><?php echo gmdate("Y-m-d H:i:s e", (string) $account->next_vesting_withdrawal / 1000) ?></td></tr>{% endif %}</table>" data-position="left center" data-variation="very wide">
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
{% endblock %}

{% block scripts %}
  {% if chart is defined %}
    {% include 'charts/account/' ~ router.getActionName() %}
  {% endif %}
  <script>
    $('#vp_progress')
      .progress({
        percent: {{ live[0]['voting_power'] / 100 }}
      });
    $('#mana_progress')
      .progress({
        percent: <?php echo round($rc['rc_manabar']['current_mana'] / $rc['max_rc'] * 100, 2);?>
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
    
    {% if router.getActionName() == "view" %}
    // load more history records infinitely
    (function() {
      var accountName = "{{ accountName }}";
      if (accountName === '') {
        console.log('accountName is empty');
        return;
      }
      var nextStart = {{ nextStart ? nextStart : 'null' }};
      var isLoading = false;
      var hasMore = {{ nextStart ? 'true' : 'false' }};
      // 75% of the scroll position to load more history records when scrolling
      var scrollThreshold = 0.75;
      
      // format time
      function formatTimeAgo(timestamp) {
        var seconds = Math.floor((new Date() - new Date(timestamp)) / 1000);
        if (seconds < 60) return seconds + ' seconds ago';
        var minutes = Math.floor(seconds / 60);
        if (minutes < 60) return minutes + ' minutes ago';
        var hours = Math.floor(minutes / 60);
        if (hours < 24) return hours + ' hours ago';
        var days = Math.floor(hours / 24);
        if (days < 7) return days + ' days ago';
        var weeks = Math.floor(days / 7);
        if (weeks < 4) return weeks + ' weeks ago';
        var months = Math.floor(days / 30);
        if (months < 12) return months + ' months ago';
        return Math.floor(days / 365) + ' years ago';
      }
      
      // format operation name
      function formatOpName(opType) {
        var names = {
          'transfer': 'Transfer',
          'vote': 'Vote',
          'comment': 'Comment',
          'author_reward': 'Author Reward',
          'curation_reward': 'Curation Reward',
          'transfer_to_vesting': 'Power Up',
          'withdraw_vesting': 'Power Down',
          'delegate_vesting_shares': 'Delegate',
          'account_update': 'Account Update',
          'custom_json': 'Custom JSON'
        };
        return names[opType] || opType.replace(/_/g, ' ').replace(/\b\w/g, function(l) { return l.toUpperCase(); });
      }
      
      // create simplified transaction display
      function createTxContent(item) {
        var op = item[1].op;
        var opType = op[0];
        var opData = op[1];
        var content = '';
        
        switch(opType) {
          case 'transfer':
            content = '<span class="ui left labeled button"><a href="/@' + opData.from + '" class="ui basic right pointing label">' + opData.from + '</a><a href="/@' + opData.to + '" class="ui button">' + opData.to + '</a></span><span class="ui green label">' + opData.amount + '</span>';
            if (opData.memo) {
              content += '<div class="ui segment" style="max-width: 620px; overflow: scroll">' + opData.memo + '</div>';
            }
            break;
          case 'vote':
            content = '<div class="ui small header"><a href="/tag/@' + opData.author + '/' + opData.permlink + '">' + opData.permlink.replace(/-/g, ' ') + '</a><div class="sub header"><div class="ui small celled horizontal list"><span class="item">↳ by: <a href="/@' + opData.author + '">@' + opData.author + '</a></span><span class="item">voter: <a href="/@' + opData.voter + '">@' + opData.voter + '</a> (' + (opData.weight / 100) + '%)</span></div></div></div>';
            break;
          default:
            content = '<span class="ui label">' + JSON.stringify(opData).substring(0, 100) + '</span>';
        }
        return content;
      }
      
      // load more history records
      function loadMoreHistory() {
        if (isLoading || !hasMore || nextStart === null) {
          return;
        }
        
        isLoading = true;
        $('#history-loading').show();
        $('#history-no-more').hide();
        
        $.ajax({
          url: '/api/history/' + accountName,
          data: {
            start: nextStart,
            limit: 20
          },
          dataType: 'json',
          success: function(response) {
            if (response.error) {
              console.error('Error loading history:', response.error);
              isLoading = false;
              $('#history-loading').hide();
              return;
            }
            
            if (response.data && response.data.length > 0) {
              var tbody = $('#history-tbody');
              response.data.forEach(function(item) {
                var op = item[1].op;
                var opType = op[0];
                var tr = $('<tr data-history-item>');
                
                var td1 = $('<td class="three wide">');
                var header = $('<div class="ui small header">').text(formatOpName(opType));
                var subHeader = $('<div class="sub header">');
                subHeader.append('<span>' + formatTimeAgo(item[1].timestamp) + '</span>');
                subHeader.append('<br><a href="/block/' + item[1].block + '"><small style="color: #bbb">Block #' + item[1].block + '</small></a>');
                header.append(subHeader);
                td1.append(header);
                
                var td2 = $('<td>').html(createTxContent(item));
                tr.append(td1).append(td2);
                tbody.append(tr);
              });
              
              nextStart = response.nextStart;
              hasMore = response.hasMore;
            } else {
              hasMore = false;
            }
            
            if (!hasMore) {
              $('#history-no-more').show();
            }
          },
          error: function(xhr, status, error) {
            console.error('Error loading history:', error);
          },
          complete: function() {
            isLoading = false;
            $('#history-loading').hide();
          }
        });
      }
      
      // scroll event listener with throttle (500ms)
      var lastScrollTime = 0;
      var throttleDelay = 500;
      
      $(window).on('scroll', function() {
        if (!hasMore || isLoading) return;
        
        var currentTime = new Date().getTime();
        
        // throttle: only execute if 500ms have passed since last execution
        if (currentTime - lastScrollTime < throttleDelay) {
          return;
        }
        
        lastScrollTime = currentTime;
        
        var windowHeight = $(window).height();
        var documentHeight = $(document).height();
        var scrollTop = $(window).scrollTop();
        var scrollPercent = scrollTop / (documentHeight - windowHeight);
        
        if (scrollPercent >= scrollThreshold) {
          loadMoreHistory();
        }
      });
    })();
    {% endif %}
  </script>
{% endblock %}
