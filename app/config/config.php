<?php

use Phalcon\Config;
use Phalcon\Logger;

return new Config([
    'database' => [
        'host' => getenv('MONGODB'),
        'dbname' => 'steemdb'
    ],
    'application' => [
        'controllersDir' => APP_PATH . '/controllers/',
        'modelsDir'      => APP_PATH . '/models/',
        'formsDir'       => APP_PATH . '/forms/',
        'viewsDir'       => APP_PATH . '/views/',
        'libraryDir'     => APP_PATH . '/library/',
        'helpersDir'     => APP_PATH . '/helpers/',
        'pluginsDir'     => APP_PATH . '/plugins/',
        'cacheDir'       => BASE_PATH . '/cache/',
        'baseUri'        => '/',
        'publicUrl'      => 'steemdb.io',
        'cryptSalt'      => 'eEAfR|_&G&f,+vU]:jFr!!A&+71w1Ms9~8_4L!<@[N@DyaIP_2My|:+.u>/6m,$D'
    ],
    'logger' => [
        'saveFile' => false,
        'path'     => BASE_PATH . '/logs/',
        'format'   => '%date% [%type%] %message%',
        'date'     => 'D j H:i:s',
        'logLevel' => Logger::DEBUG,
        'filename' => 'application.log',
    ],
    'steemd' => [
        'url'       => getenv('STEEMD_URL') ? getenv('STEEMD_URL') : 'https://api.steemit.com',
    ],
]);
