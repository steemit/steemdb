<div class="ui three small secondary statistics">
  <div class="statistic">
    <div class="value">
      {{ account.followers | length }}
    </div>
    <div class="label">
      Followers
    </div>
  </div>
  <div class="statistic">
    <div class="value">
      {{ account.post_count ? account.post_count : 0 }}
    </div>
    <div class="label">
      Posts
    </div>
  </div>
  <div class="statistic">
    <div class="value">
      {{ account.following | length }}
    </div>
    <div class="label">
      Following
    </div>
  </div>
</div>
<h3 class="ui header">
  Recent History
  <div class="sub header">
    All recent activity involving @{{ account.name }}.
  </div>
</h3>

<table class="ui stackable definition table" id="history-table">
  <thead></thead>
  <tbody id="history-tbody">
  {% for item in activity %}
  <tr data-history-item>
    <td class="three wide">
      <div class="ui small header">
        <?php echo $this->opName::string($item[1]['op'], $account) ?>
        <div class="sub header">
          <?php echo $this->timeAgo::string($item[1]['timestamp']); ?>
          <br><a href="/block/{{ item[1]['block' ]}}"><small style="color: #bbb">Block #{{ item[1]['block' ]}}</small></a>
        </div>
      </div>
    </td>
    <td>
      <?php
      // Get template name and validate it to prevent path traversal attacks
      $template_name = isset($item[1]['op'][0]) ? $item[1]['op'][0] : '';
      
      // Sanitize: only allow alphanumeric characters, underscores, and hyphens
      // Remove any path traversal attempts (../, ..\, etc.)
      $template_name = preg_replace('/[^a-zA-Z0-9_-]/', '', $template_name);
      
      // If empty after sanitization, use unknown template
      if (empty($template_name)) {
        $template_to_include = 'unknown';
      } else {
        // Build safe path using basename to prevent any remaining path traversal
        $template_path = $this->getViewsDir() . '_elements/tx/' . basename($template_name) . '.volt';
        $template_to_include = file_exists($template_path) ? basename($template_name) : 'unknown';
      }
      
      $this->view->partial('_elements/tx/' . $template_to_include, ['item' => $item]);
      ?>
    </td>
  </tr>
  {% else %}
  <tr>
    <td>
      Unable to connect to steemd for to load recent history.
    </td>
  </tr>
  {% endfor %}
  </tbody>
</table>
<div id="history-loading" style="display: none; text-align: center; padding: 20px;">
  <div class="ui active inline loader"></div>
  <span>Loading more history...</span>
</div>
<div id="history-no-more" style="display: none; text-align: center; padding: 20px; color: #999;">
  No more history records.
</div>
