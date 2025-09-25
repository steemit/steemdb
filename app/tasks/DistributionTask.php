<?php
namespace SteemDB\Tasks;

use Phalcon\Cli\Task;

class DistributionTask extends Task
{
    // default action if no action gived in command
    public function mainAction()
    {
        echo "This is the default action of HelloTask\n";
    }
}