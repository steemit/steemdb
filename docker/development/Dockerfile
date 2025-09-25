FROM ubuntu:16.04

ENV php_conf /etc/php/7.0/fpm/php.ini
ENV fpm_conf /etc/php/7.0/fpm/php-fpm.conf
ENV composer_hash e0012edf3e80b6978849f5eff0d4b4e4c79ff1609dd1e613307e16318854d24ae64f26d17af3ef0bf7cfb710ca74755a
ENV MONGODB mongodb://172.31.31.104:27017
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update -y && apt-get install -y software-properties-common python-software-properties && \
    apt-add-repository -y ppa:phalcon/stable && apt-get update -y && \
    apt-get install -y bash \
    cron \
    openssh-client \
    wget \
    nginx \
    supervisor \
    curl \
    git \
    php7.0-dev \
    php7.0-fpm \
    php7.0-mcrypt \
    php7.0-ctype \
    php7.0-gd \
    php7.0-intl \
    php7.0-xml \
    php7.0-xsl \
    php7.0-curl \
    php7.0-iconv \
    php7.0-json \
    php7.0-phar \
    php7.0-dom \
    php7.0-phalcon \
    php7.0-zip \
    php-xdebug \
    php7.0-mbstring \
    ca-certificates \
    dialog \
    gcc \
    musl-dev \
    libffi-dev &&\
    mkdir -p /etc/nginx && \
    mkdir -p /var/www/app && \
    mkdir -p /run/nginx && \
    mkdir -p /var/log/supervisor && \
    php -r "copy('https://getcomposer.org/installer', 'composer-setup.php');" && \
    #php -r "if (hash_file('SHA384', 'composer-setup.php') === '${composer_hash}') { echo 'Installer verified'; } else { echo 'Installer corrupt'; unlink('composer-setup.php'); } echo PHP_EOL;" && \
    php composer-setup.php --install-dir=/usr/bin --filename=composer && \
    php -r "unlink('composer-setup.php');" && \
    # nginx site conf
    #mkdir -p /etc/nginx/sites-available/ && \
    #mkdir -p /etc/nginx/sites-enabled/ && \
    #mkdir -p /etc/nginx/ssl/ && \
    #rm -Rf /var/www/* && \
    #ln -s /etc/nginx/sites-available/default.conf /etc/nginx/sites-enabled/default.conf && \
    apt-get update -y && apt-get install -y php-pear libcurl4-openssl-dev pkg-config libssl-dev libsslcommon2-dev && \
    pecl install mongodb-1.9.2 && \
    echo "extension=mongodb.so" > /etc/php/7.0/fpm/conf.d/30-mongo.ini && \
    echo "extension=mongodb.so" > /etc/php/7.0/cli/conf.d/30-mongo.ini && \
    apt-get install -y memcached php-memcached && \
    apt-get remove -y gcc musl-dev && apt-get clean -y

# tweak php-fpm config
RUN mkdir -p /run/php &&\
    sed -i -e "s/;cgi.fix_pathinfo=1/cgi.fix_pathinfo=0/g" ${php_conf} && \
    sed -i -e "s/upload_max_filesize\s*=\s*2M/upload_max_filesize = 100M/g" ${php_conf} && \
    sed -i -e "s/post_max_size\s*=\s*8M/post_max_size = 100M/g" ${php_conf} && \
    sed -i -e "s/variables_order = \"GPCS\"/variables_order = \"EGPCS\"/g" ${php_conf} && \
    sed -i -e "s/;daemonize\s*=\s*yes/daemonize = no/g" ${fpm_conf} && \
    sed -i -e "s/;catch_workers_output\s*=\s*yes/catch_workers_output = yes/g" ${fpm_conf} && \
    sed -i -e "s/pm.max_children = 4/pm.max_children = 4/g" ${fpm_conf} && \
    sed -i -e "s/memory_limit = 128M/memory_limit = 512M/" ${php_conf} &&\
    ln -s ${php_conf} /etc/php/7.0/fpm/conf.d/php.ini && \
    find /etc/php/7.0/fpm/conf.d/ -name "*.ini" -exec sed -i -re 's/^(\s*)#(.*)/\1;\2/g' {} \; && \
    sed -i -e "s/memory_limit = 128M/memory_limit = 512M/" /etc/php/7.0/fpm/conf.d/php.ini &&\
    echo "extension=phalcon.so" > /etc/php/7.0/fpm/conf.d/30-phalcon.ini && \
    echo "extension=phalcon.so" > /etc/php/7.0/cli/conf.d/30-phalcon.ini

# Add Scripts
ADD ./docker/development/scripts/start.sh /start.sh
RUN chmod 755 /start.sh

# Copy our nginx and php-fpm config
RUN rm -Rf /etc/nginx/nginx.conf
ADD ./docker/development/conf/nginx.conf /etc/nginx/nginx.conf
COPY ./docker/development/conf/vhost.conf /etc/nginx/sites-enabled/default
COPY ./docker/development/conf/www.conf /etc/php/7.0/fpm/pool.d/www.conf

# Services
ADD ./docker/development/conf/supervisord.conf /etc/supervisord.conf

COPY . /var/www/html
RUN composer -d/var/www/html install && \
    chmod -R 777 /var/www/html/cache && \
    chmod -R 777 /var/www/html/logs

ADD ./rds-combined-ca-bundle.pem /src/rds-combined-ca-bundle.pem

# Config Crontab
RUN echo "/usr/bin/php /var/www/html/cli.php Distribution update" > /usr/bin/run-php-cli.sh && \
    chmod +x /usr/bin/run-php-cli.sh && \
    echo '*/10 * * * * /usr/bin/run-php-cli.sh >> /dev/stdout 2>&1' > /etc/cron.d/cronjob && \
    crontab /etc/cron.d/cronjob

EXPOSE 443 8000

CMD ["/start.sh"]
