<table class="ui definition table" style="table-layout: fixed">
  <tbody>
    <?php foreach ($data as $key => $value) { ?>
    <tr>
      <td class="three wide">
        <small><?= $key ?></small>
      </td>
      <td>
        <?php
          switch(gettype($value)) {
            case "array":
            case "object":
              if($value instanceOf MongoDB\BSON\UTCDateTime) {
                echo $value->toDateTime()->format('Y-m-d H:i');
              } else {
                echo "<pre>" . json_encode($value, JSON_PRETTY_PRINT) . "</pre>";
              }
              break;
            case "double":
              echo number_format($value, 3, '.', ',');
              break;
            default:
              echo htmlspecialchars($value);
              break;
          }
        ?>
      </td>
    </tr>
    <?php } ?>
  </tbody>
</table>
