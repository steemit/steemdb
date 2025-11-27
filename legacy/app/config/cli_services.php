<?php
use Phalcon\Crypt;
use Phalcon\Db\Adapter\MongoDB\Client;
use Phalcon\Events\Manager as EventsManager;
use Phalcon\Flash\Direct as Flash;
use Phalcon\Logger\Adapter\File as FileLogger;
use Phalcon\Logger\Formatter\Line as FormatterLine;
use Phalcon\Mvc\Collection\Manager;
use Phalcon\Cli\Dispatcher as CliDispatcher;
use Phalcon\Cli\Dispatcher\Exception as DispatchException;
use Phalcon\Cli\Model\Metadata\Files as MetaDataAdapter;

/**
 * Register the global configuration as config
 */
$di->setShared('config', function () {
    return include APP_PATH . "/config/config.php";
});

/**
 * Database connection is created based in the parameters defined in the configuration file
 */
$di->setShared('mongo', function () {
  $config = $this->getConfig();
  $mongoConfig = $config->database->host;
  $tmp = explode('/?', $mongoConfig);
  if ($tmp[1] && strpos($tmp[1], 'ssl=true') !== false) {
    // enable ssl
    preg_match('/ssl_ca_certs=([^&]+)/', $tmp[1], $matches);
    if ($matches[1]) {
      $SSL_FILE = $matches[1];
      //Specify the Amazon DocumentDB cert
      $ctx = stream_context_create(array(
          "ssl" => array(
              "cafile" => $SSL_FILE,
          ))
      );
      $mongo = new Client($tmp[0], array("ssl" => true), array("context" => $ctx));
    } else {
      var_dump($matches);
      echo 'ssl error';
      die();
    }
  } else {
    // disable ssl
    $mongo = new Client($config->database->host);
  }
  $options = [];
  return $mongo->selectDatabase($config->database->dbname, $options);
});

// Collection Manager is required for MongoDB
$di->setShared('collectionManager', function () {
  $manager = new Manager();
  return $manager;
});

/**
 * If the configuration specify the use of metadata adapter use it or use memory otherwise
 */
$di->set('modelsMetadata', function () {
  $config = $this->getConfig();
  return new MetaDataAdapter([
    'metaDataDir' => $config->application->cacheDir . 'cliMetaData/'
  ]);
});

/**
 * Crypt service
 */
$di->set('crypt', function () {
    $config = $this->getConfig();
    $crypt = new Crypt();
    $crypt->setKey($config->application->cryptSalt);
    return $crypt;
});

/**
 * Dispatcher use a default namespace
 */
$di->set('dispatcher', function () {
  $dispatcher = new CliDispatcher();
  $dispatcher->setDefaultNamespace('SteemDB\Tasks');
  return $dispatcher;
});

/**
 * Logger service
 */
$di->set('logger', function ($filename = null, $format = null) {
  $config = $this->getConfig();

  $format   = $format ?: $config->get('logger')->format;
  $filename = trim($filename ?: $config->get('logger')->filename, '\\/');
  $path     = rtrim($config->get('logger')->path, '\\/') . DIRECTORY_SEPARATOR;
  $saveFile = $config->get('logger')->saveFile;

  $formatter = new FormatterLine($format, $config->get('logger')->date);
  if ($saveFile) {
    $logger    = new FileLogger($path . $filename);
  } else {
    $logger    = new FileLogger('php://stdout');
  }

  $logger->setFormatter($formatter);
  $logger->setLogLevel($config->get('logger')->logLevel);

  return $logger;
});

$di->set('steemd', function() {
  $config = $this->getConfig();
  require_once(APP_PATH . '/libs/steemd.php');
  return new Steemd($config->get('steemd')->url);
});

$di->set('memcached', function() {
  $frontendOptions = array(
    'lifetime' => 60 * 5
  );
  $frontCache = new \Phalcon\Cache\Frontend\Data($frontendOptions);
  $backendOptions = array(
    "servers" => array(
      array(
        'host' => 'localhost',
        'port' => 11211,
        'weight' => 1
      ),
    )
  );
  $cache = new \Phalcon\Cache\Backend\Libmemcached($frontCache, $backendOptions);
  return $cache;
});

$di->set('util', function() {
  require_once(APP_PATH . '/libs/utilities.php');
  return new Utilities($this);
});

$di->set('convert', function () { return new SteemDB\Helpers\Convert(); });
$di->set('largeNumber', function () { return new SteemDB\Helpers\LargeNumber(); });
$di->set('reputation', function () { return new SteemDB\Helpers\Reputation(); });
$di->set('timeAgo', function () { return new SteemDB\Helpers\TimeAgo(); });
$di->set('opName', function () { return new SteemDB\Helpers\OpName(); });
