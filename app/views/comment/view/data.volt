{% if comment %}
<a class="ui primary button" href="/{{ comment.category }}/@{{ comment.author }}/{{ comment.permlink }}/json">View JSON</a>
{% include '_elements/definition_table' with ['data': comment.toArray()] %}
{% else %}
<div class="ui error message">
  Comment not found.
</div>
{% endif %}
