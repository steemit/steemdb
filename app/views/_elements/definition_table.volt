<table class="ui definition table" style="table-layout: fixed">
  <tbody>
    {% for key, value in data %}
    <tr>
      <td class="three wide">
        <small>{{ key }}</small>
      </td>
      <td class="wide">
        <?php
          switch(gettype($value)) {
            case "array":
            case "object":
              if($value instanceOf MongoDB\BSON\UTCDateTime) {
                echo $value->toDateTime()->format('Y-m-d H:i');
              } else {
                echo "<pre>" . htmlspecialchars(json_encode($value, JSON_PRETTY_PRINT)) . "</pre>";
              }
              break;
            case "double":
              echo number_format($value, 3, '.', ',');
              break;
            default:
              if (in_array($key, ['creator', 'delegator', 'delegatee'])) {
                echo '<a href="/@'.htmlspecialchars($value).'">'.htmlspecialchars($value).'</a>';
              } else {
                echo htmlspecialchars($value);
              }
              break;
          }
        ?>
      </td>
    </tr>
    {% endfor %}
  </tbody>
</table>
