<?php
namespace SteemDB\Controllers;

use SteemDB\Models\Account;
use SteemDB\Models\Comment;

use MongoDB\BSON\Regex;

class SearchController extends ControllerBase
{
  public function indexAction()
  {
    $query = $this->request->get("q", "string");
    $data = [
      'results' => [],
    ];

    if (is_numeric($query)) {
      // block search
      $block = $this->steemd->getBlock($query);
      $data['results']['block'] = [
        'name' => 'Block',
        'results' => $block ? [[
          'title' => $query,
          'url' => '/block/'.$query,
        ]] : [],
      ];
    } else if (strlen($query) == 40) {
      // transaction search
      $trx = $this->steemd->getTx($query);
      $data['results']['trx'] = [
        'name' => 'Transaction',
        'results' => $trx ? [[
          'title' => $trx['transaction_id'],
          'url' => '/tx/'.$trx['transaction_id'],
        ]] : [],
      ];
    } else {
      // user search
      $accounts = Account::find(array(
      array(
          'name' => new Regex('^'.$query, 'i')
        ),
        "sort" => [
          "followers" => -1
        ],
        "fields" => [
          'name' => 1
        ],
        "limit" => 5
      ));
      $data['results']['accounts'] = [
        'name' => 'Accounts',
        'results' => array_map(function($account) {
          return [
            'title' => $account->name,
            'url' => '/@'.$account->name
          ];
        }, $accounts),
      ];
    }
    $this->view->disable();
    $this->response->setContentType('application/json', 'UTF-8');
    $this->response->setJsonContent($data);
    return $this->response;
  }
  public function pageAction()
  {
    $this->view->q = $q = $this->request->get("q");
    if($q) {
      $this->view->results = $results = Comment::agg(array(
        [ '$match' => [ '$text' => [ '$search' => $q ] ] ],
        [ '$sort' => [ 'score' => [ '$meta' => "textScore" ] ] ],
        [ '$limit' => 100 ],
        // [ '$project' => [ 'title' => 1, '_id' => 0 , '_ts' => 1] ],
      ));
    }

  }
}
