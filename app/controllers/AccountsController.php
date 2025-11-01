<?php
namespace SteemDB\Controllers;

use SteemDB\Models\Account;
use SteemDB\Models\Statistics;

class AccountsController extends ControllerBase
{

  public function listAction()
  {
    $this->view->filter = $filter = $this->dispatcher->getParam("filter", "string");
    $this->view->page = $page = (int) $this->request->get("page") ?: 1;
    $query = array();
    $sort = array(
      "vesting_shares" => -1,
    );
    if($filter) {
      switch($filter) {
        case "powerdown":
          $sort = array(
            "vesting_withdraw_rate" => -1
          );
          break;
        case "followers_mvest":
          $sort = array(
            "followers_mvest" => -1,
          );
          break;
        case "sbd":
          $sort = array(
            "total_sbd_balance" => -1,
          );
          break;
        case "steem":
          $sort = array(
            "total_balance" => -1,
          );
          break;
        case "reputation":
          $query['reputation'] = array('$gt' => 0);
          $sort = array(
            "reputation" => -1,
          );
          break;
        case "posts":
          $sort = array(
            "post_count" => -1,
          );
          break;
        case "followers":
          $sort = array(
            "followers_count" => -1,
          );
          break;
        default:
          break;
      }
    }
    $limit = 10;
    // Determine how many pages of users we have
    // echo 'start';
    // print_r(array(
    //   array('key' => 'users'),
    //   "sort" => array('date' => -1)
    // ));
    // echo(stop);
    $stats = Statistics::findFirst(array(
      array('key' => 'users'),
      "sort" => array('date' => -1)
    ));
    
    // Check if statistics record exists
    if ($stats) {
      $this->view->pages = ceil($stats->toArray()['value'] / $limit);
    } else {
      // Default to 1 page if statistics not found
      $this->view->pages = 1;
    }
    
    // Load the accounts
    $this->view->accounts = Account::find(array(
      $query,
      "skip" => $limit * ($page - 1),
      "sort" => $sort,
      "limit" => $limit
    ));
  }

}
