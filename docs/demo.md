# TCP Routing demo
This page gives step by step instructions to do the TCP routing demo as done in Cloud Foundry Summit 2016.

The demo at Cloud Foundry Summit 2016 was to push MQTT broker (`mosquitto`) as CF app and create and bind a TCP route to it. To showcase that MQTT broker running as CF app is accessible for publishing and subscribing using TCP route, a subscriber web app and publisher android app was used.

## Pushing MQTT broker as CF app
We will be using [mosquitto](http://mosquitto.org/) broker's docker image to push mqtt broker as CF app. We will use [toke/mosquitto](https://github.com/toke/docker-mosquitto) docker image.

1. Create a tcp domain if you don't have one.

    To discover whether you have a TCP domain available:
    ```
    cf domains
    Getting domains in org my-org as my-user...
    name                                       status   type
    example.com                                shared
    tcp.example.com                            shared   tcp
    ```
    
    If you don't find a domain of type tcp, create one as admin user:
    ```
    cf create-shared-domain <tcp-domain> --router-group default-tcp
    ```
   Replace `<tcp-domain>` with the name of the domain you want to give to your tcp domain. Of course, here we are assuming that dns entries for above created tcp domain will resolve to load balancer in front of tcp router groups or if tcp routers have public IP addresses then it will resolve to tcp routers. This has to be done by the operator/administrator of your Cloud Foundry deployment.

1. Push mosquitto as CF app as follows:
    ```
    cf push mqttbroker --docker-image toke/mosquitto -d <tcp-domain> --random-route
    ```
    This will download the required docker image and push it as CF app and create and bind a tcp route.

1. Now you are ready to supply this tcp route information to your publisher and subscriber.

## Pushing MQTT subscriber app
This web app subscribes to `accelerometer` topic of MQTT broker and displays the data published on this topic in a chart. It expects numeric data to be published on this topic.

1. Fetch the repo for [sample-mqtt-subscriber](https://github.com/GESoftware-CF/sample-mqtt-subscriber)
    ```
    mkdir -p ~/workspace
    cd ~/workspace
    git clone https://github.com/GESoftware-CF/sample-mqtt-subscriber.git
    cd sample-mqtt-subscriber/
    ```

1. Build the project as follows.
    
    Install maven if you don't already have it. Maven require java:

    ```
    brew install Caskroom/cask/java
    brew install maven
    ```
    
    Once you have maven installed:
    ```
    mvn clean package
    ```
    
    

1. Push the resulting jar as CF app
    ```
    cf push mqttsub -p target/mqttsubscriber-0.0.1-SNAPSHOT.jar
    ```

1. Verify that the app is successfully deployed and running by hitting endpoint `mqttsub.<app-domain>`. You should see web page like this:

![Image of mqttsubscriber landing page]
(images/mqttsub_landing_page.png)

## Installing MQTT publisher android app
This android app registers itself to accelerometer sensor of the device and publishes y-axis acceleration to `accelerometer` topic of MQTT broker. 

1. Fetch the repo for [sample-android-mqtt](https://github.com/atulkc/sample-android-mqtt)
    ```
    mkdir -p ~/workspace
    cd ~/workspace
    git clone https://github.com/atulkc/sample-android-mqtt.git
    cd sample-android-mqtt/
    ```

1. Load the project in android studio.

1. Install the application on your favorite android phone using android studio.

1. Verify that the app is successfully installed and running by tapping the `Accelerometer` icon on your smartphone. You should see something like this:

    <img src="images/android_landing_page.png" alt="Image of android app landing page" width="200px" height="300px"/>


## Putting it all together
This is where we tie together the subscriber web app, android app and mqtt broker.

1. Enter `<tcp-domain>` as host name on landing page of web app and port of tcp route as `port`. You should see an empty chart as follows:

    ![Image of mqttsubscriber landing page]
    (images/mqttsub_empty_chart_page.png)

1. Now enter same information on android accelerometer app. You should see the next screen on android app indicating the y-axis acceleration:

    <img src="images/android_connected_page.png" alt="Image of android app connected page" width="200px" height="300px"/>

1. The y-axis acceleration of your device should now start showing up on chart on web app:

	![Image of chart of web app]
	(images/mqttsub_chart_page.png)

