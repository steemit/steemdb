<?php

use Phalcon\Loader;

$loader = new Loader();

$loader->registerNamespaces([
    'SteemDB\Tasks'       => $config->application->tasksDir,
    'SteemDB\Models'      => $config->application->modelsDir,
    'SteemDB\Helpers'     => $config->application->helpersDir,
]);

$loader->registerDirs(array(
    '../app/helpers'
));

$loader->register();

// Use composer autoloader to load vendor classes
require_once BASE_PATH . '/vendor/autoload.php';
