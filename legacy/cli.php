<?php
use Phalcon\Exception;
use Phalcon\Cli\Console;
use Phalcon\Di\FactoryDefault\Cli as CliDI;

error_reporting(E_ERROR | E_PARSE);
define('BASE_PATH', __DIR__);
define('APP_PATH', BASE_PATH . '/app');

$arguments = [];
foreach ($argv as $k => $arg) {
    if ($k === 1) {
        $arguments['task'] = $arg;
    } elseif ($k === 2) {
        $arguments['action'] = $arg;
    } elseif ($k >= 3) {
        $arguments['params'][] = $arg;
    }
}

try {
    $di = new CliDI();

    include APP_PATH . "/config/cli_services.php";

    $config = $di->getConfig();

    include APP_PATH . '/config/cli_loader.php';

    $console = new Console($di);
    $console->handle($arguments);
} catch (Exception $exception) {
    fwrite(STDERR, $exception->getMessage() . PHP_EOL);
    exit(1);
}